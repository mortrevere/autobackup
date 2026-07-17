package autobackup

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const maxInitialWorkerJitter = 5 * time.Second

var jobHeartbeatInterval = 30 * time.Second

type Runner struct {
	Stdout io.Writer
	Stderr io.Writer
}

type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (w lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}

type Result struct {
	ChangedByLocation map[string]int
	Failures          []error
}

func (r Runner) Run(ctx context.Context, plan Plan) Result {
	if r.Stdout == nil {
		r.Stdout = io.Discard
	}
	if r.Stderr == nil {
		r.Stderr = r.Stdout
	}
	outputMu := &sync.Mutex{}
	r.Stdout = lockedWriter{mu: outputMu, w: r.Stdout}
	r.Stderr = lockedWriter{mu: outputMu, w: r.Stderr}
	jobs := plan.Jobs
	if jobs < 1 {
		jobs = 1
	}
	start := time.Now()
	if !plan.Quiet {
		fmt.Fprintln(r.Stdout, Colorize(ColorWhite, fmt.Sprintf("Starting %d rsync task(s) with %d worker(s)", len(plan.Tasks), jobs)))
		r.printSplitModes(plan)
	}
	identityOutput := r.Stdout
	if plan.Quiet {
		identityOutput = io.Discard
	}
	if err := PrepareIdentityFile(ctx, plan.Config.Destination.IdentityFile, identityOutput); err != nil {
		return Result{
			ChangedByLocation: map[string]int{},
			Failures:          []error{err},
		}
	}

	tasks := make(chan Task)
	results := make(chan taskResult)
	var activeTasks atomic.Int64
	var enqueuedTasks atomic.Int64
	enqueuedTasks.Store(int64(len(plan.Tasks)))
	stopHeartbeat := r.startJobHeartbeat(ctx, &activeTasks, &enqueuedTasks)
	var wg sync.WaitGroup
	for i := 0; i < jobs; i++ {
		wg.Add(1)
		go func(workerIndex int) {
			defer wg.Done()
			if workerIndex > 0 && len(plan.Tasks) > 1 {
				if !sleepWithContext(ctx, rand.N(maxInitialWorkerJitter)) {
					return
				}
			}
			for task := range tasks {
				enqueuedTasks.Add(-1)
				activeTasks.Add(1)
				res := r.runTask(ctx, plan, task)
				activeTasks.Add(-1)
				results <- res
			}
		}(i)
	}
	go func() {
		defer stopHeartbeat()
		for _, task := range plan.Tasks {
			select {
			case <-ctx.Done():
				close(tasks)
				wg.Wait()
				close(results)
				return
			case tasks <- task:
			}
		}
		close(tasks)
		wg.Wait()
		close(results)
	}()

	out := Result{ChangedByLocation: map[string]int{}}
	for res := range results {
		out.ChangedByLocation[res.LocationName] += res.Changed
		if res.Err != nil {
			out.Failures = append(out.Failures, res.Err)
		}
	}
	if err := ctx.Err(); err != nil {
		out.Failures = append(out.Failures, fmt.Errorf("backup interrupted: %w", err))
	}

	fmt.Fprintln(r.Stdout, Colorize(ColorGreen, fmt.Sprintf("Done in %s", time.Since(start).Round(time.Second))))
	r.printChangedSummary(plan, out.ChangedByLocation)
	r.printDiskSummary(ctx, plan)
	return out
}

func (r Runner) printSplitModes(plan Plan) {
	summaries := map[string]*splitModeSummary{}
	var order []string
	for _, task := range plan.Tasks {
		if task.SplitMode == SplitExplicit {
			continue
		}
		summary := summaries[task.LocationName]
		if summary == nil {
			summary = &splitModeSummary{}
			summaries[task.LocationName] = summary
			order = append(order, task.LocationName)
		}
		switch {
		case task.Kind == TaskRootFilesOnly:
			summary.RootFiles = true
		case task.SplitMode == SplitHeuristicRecursive:
			summary.Recursive++
		case task.SplitMode == SplitHeuristicSplit:
			summary.Split++
		}
	}
	for _, loc := range order {
		fmt.Fprintln(r.Stdout, Colorize(ColorBlue, fmt.Sprintf("%s process splitting: %s", LogPrefix(loc), summaries[loc].String())))
	}
}

type splitModeSummary struct {
	Split     int
	Recursive int
	RootFiles bool
}

func (s splitModeSummary) String() string {
	var parts []string
	if s.Split > 0 && s.Recursive == 0 && !s.RootFiles {
		return fmt.Sprintf("split-rsync (heuristic: %d split)", s.Split)
	}
	if s.Recursive > 0 && s.Split == 0 && !s.RootFiles {
		return fmt.Sprintf("recursive-rsync (heuristic: %d recursive)", s.Recursive)
	}
	if s.Split > 0 {
		parts = append(parts, fmt.Sprintf("%d split", s.Split))
	}
	if s.Recursive > 0 {
		parts = append(parts, fmt.Sprintf("%d recursive", s.Recursive))
	}
	if s.RootFiles {
		parts = append(parts, "root-files")
	}
	return "split-rsync (heuristic: " + strings.Join(parts, ", ") + ")"
}

func (r Runner) startJobHeartbeat(ctx context.Context, activeTasks, enqueuedTasks *atomic.Int64) func() {
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(jobHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				active := activeTasks.Load()
				enqueued := enqueuedTasks.Load()
				if active <= 0 && enqueued <= 0 {
					return
				}
				fmt.Fprintln(r.Stdout, Colorize(ColorBlue, fmt.Sprintf("still running: %d rsync jobs, %d enqueued", active, enqueued)))
			}
		}
	}()
	return func() {
		close(done)
		<-stopped
	}
}

func (r Runner) printChangedSummary(plan Plan, changed map[string]int) {
	fmt.Fprintln(r.Stdout)
	fmt.Fprintln(r.Stdout, Colorize(ColorWhite, "Files changed"))
	fmt.Fprintln(r.Stdout, Colorize(ColorWhite, "-------------"))
	for _, loc := range locationNames(plan) {
		count, ok := changed[loc]
		if !ok {
			continue
		}
		fmt.Fprintln(r.Stdout, Colorize(ColorWhite, fmt.Sprintf("%6d  %s", count, loc)))
	}
	fmt.Fprintln(r.Stdout)
}

func locationNames(plan Plan) []string {
	seen := make(map[string]bool, len(plan.Config.Locations))
	names := make([]string, 0, len(plan.Config.Locations))
	for _, loc := range plan.Config.Locations {
		if !seen[loc.Source] {
			names = append(names, loc.Source)
			seen[loc.Source] = true
		}
	}
	for _, task := range plan.Tasks {
		if !seen[task.LocationName] {
			names = append(names, task.LocationName)
			seen[task.LocationName] = true
		}
	}
	return names
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

type taskResult struct {
	LocationName string
	Changed      int
	Err          error
}

func (r Runner) runTask(ctx context.Context, plan Plan, task Task) taskResult {
	prefix := LogPrefix(task.SourceFolder)
	if plan.DryRun {
		if !plan.Quiet {
			fmt.Fprintln(r.Stdout, Colorize(ColorWhite, fmt.Sprintf("%s dry-run: skipping remote mkdir %s", prefix, task.RemoteFolder)))
		}
	} else {
		if !plan.Quiet {
			fmt.Fprintln(r.Stdout, Colorize(ColorWhite, fmt.Sprintf("%s creating %s", prefix, task.RemoteFolder)))
		}
		mkdirOutput := r.Stdout
		if plan.Quiet {
			mkdirOutput = io.Discard
		}
		if err := runCommand(ctx, plan.Tools.SSH, task.MkdirArgs, mkdirOutput); err != nil {
			return taskResult{LocationName: task.LocationName, Err: fmt.Errorf("%s mkdir failed: %w", task.SourceFolder, err)}
		}
	}
	if !plan.Quiet {
		fmt.Fprintln(r.Stdout, Colorize(ColorWhite, fmt.Sprintf("%s start", prefix)))
	}
	changed, err := r.runRsync(ctx, plan.Tools.Rsync, task.RsyncArgs, nil, prefix, plan.Quiet)
	if err != nil {
		return taskResult{LocationName: task.LocationName, Changed: changed, Err: fmt.Errorf("%s rsync failed: %w", task.SourceFolder, err)}
	}
	if !plan.DryRun {
		if err := r.verifyTask(ctx, plan, task, prefix); err != nil {
			return taskResult{LocationName: task.LocationName, Changed: changed, Err: err}
		}
	}
	return taskResult{LocationName: task.LocationName, Changed: changed}
}

func (r Runner) runRsync(ctx context.Context, bin string, args []string, stdin io.Reader, prefix string, quiet bool) (int, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	prepareCommand(cmd)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	stopCancelWatcher := watchCommandCancel(ctx, cmd)
	defer stopCancelWatcher()
	changed := 0
	lastPercent := ""
	sawProgress := false
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.Contains(line, "%"):
			fields := strings.Fields(line)
			if len(fields) > 1 {
				percent := fields[1]
				sawProgress = true
				if !quiet && percent != lastPercent {
					fmt.Fprintln(r.Stdout, Colorize(ColorYellow, fmt.Sprintf("%s %s", prefix, percent)))
				}
				lastPercent = percent
			}
		case strings.HasPrefix(line, "sent ") && strings.Contains(line, "received "):
			if !quiet {
				fmt.Fprintln(r.Stdout, Colorize(ColorGreen, fmt.Sprintf("%s %s", prefix, line)))
			}
		case strings.HasPrefix(line, "total size is ") && strings.Contains(line, "speedup "):
			if !quiet {
				fmt.Fprintln(r.Stdout, Colorize(ColorGreen, fmt.Sprintf("%s %s", prefix, line)))
			}
		case strings.HasPrefix(line, "rsync"):
			fmt.Fprintln(r.Stdout, Colorize(ColorRed, fmt.Sprintf("%s ERROR ? : %s", prefix, line)))
		case strings.HasPrefix(line, "<"):
			changed++
			if !quiet {
				fmt.Fprintln(r.Stdout, Colorize(ColorWhite, fmt.Sprintf("%s %s", prefix, line)))
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return changed, err
	}
	if err := waitCommand(ctx, cmd); err != nil {
		return changed, err
	}
	if !quiet && sawProgress && lastPercent != "100%" {
		fmt.Fprintln(r.Stdout, Colorize(ColorYellow, fmt.Sprintf("%s 100%%", prefix)))
	}
	return changed, nil
}

func runCommand(ctx context.Context, bin string, args []string, out io.Writer) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	prepareCommand(cmd)
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		return err
	}
	return waitCommand(ctx, cmd)
}

func waitCommand(ctx context.Context, cmd *exec.Cmd) error {
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		terminateCommand(cmd)
		if err := <-done; err != nil {
			return err
		}
		return ctx.Err()
	}
}

func watchCommandCancel(ctx context.Context, cmd *exec.Cmd) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			terminateCommand(cmd)
		case <-done:
		}
	}()
	return func() {
		close(done)
	}
}

func (r Runner) printDiskSummary(ctx context.Context, plan Plan) {
	fmt.Fprintln(r.Stdout, Colorize(ColorWhite, "Remote disk usage"))
	fmt.Fprintln(r.Stdout, Colorize(ColorWhite, "-----------------"))
	base := SSHArgs(plan.Config.Destination)
	target := fmt.Sprintf("%s@%s", plan.Config.Destination.Username, plan.Config.Destination.Host)
	args := append([]string{}, base...)
	args = append(args, target, "df -h "+shellQuote(plan.Config.Destination.BasePath))
	output, err := exec.CommandContext(ctx, plan.Tools.SSH, args...).CombinedOutput()
	if err != nil {
		fmt.Fprintln(r.Stdout, Colorize(ColorYellow, "disk summary unavailable"))
		return
	}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		fmt.Fprintln(r.Stdout, scanner.Text())
	}
	fmt.Fprintln(r.Stdout)
}
