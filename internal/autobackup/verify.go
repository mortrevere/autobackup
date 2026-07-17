package autobackup

import (
	"bufio"
	"container/heap"
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type VerificationMode string

const (
	VerifyChanged VerificationMode = "changed"
	VerifyAudit   VerificationMode = "audit"
	VerifyFull    VerificationMode = "full"
)

const auditMaxFilesPerTask = 128

func (r Runner) verifyTask(ctx context.Context, plan Plan, task Task, prefix string) error {
	mode := VerificationMode(task.Location.Verification)
	switch mode {
	case VerifyChanged:
		if !plan.Quiet {
			fmt.Fprintln(r.Stdout, Colorize(ColorWhite, fmt.Sprintf("%s verification changed: using rsync transfer result", prefix)))
		}
		return nil
	case VerifyAudit:
		files, err := auditFiles(task)
		if err != nil {
			return fmt.Errorf("%s audit verification failed: %w", task.SourceFolder, err)
		}
		if len(files) == 0 {
			if !plan.Quiet {
				fmt.Fprintln(r.Stdout, Colorize(ColorWhite, fmt.Sprintf("%s verification audit: no regular files selected", prefix)))
			}
			return nil
		}
		if !plan.Quiet {
			fmt.Fprintln(r.Stdout, Colorize(ColorWhite, fmt.Sprintf("%s verification audit: checksumming %d sampled file(s)", prefix, len(files))))
		}
		return r.runVerificationRsync(ctx, plan.Tools.Rsync, task, files, prefix, plan.Quiet)
	case VerifyFull:
		if !plan.Quiet {
			fmt.Fprintln(r.Stdout, Colorize(ColorWhite, fmt.Sprintf("%s verification full: checksumming all files", prefix)))
		}
		return r.runVerificationRsync(ctx, plan.Tools.Rsync, task, nil, prefix, plan.Quiet)
	default:
		return fmt.Errorf("%s unsupported verification mode %q", task.SourceFolder, mode)
	}
}

func (r Runner) runVerificationRsync(ctx context.Context, bin string, task Task, files []string, prefix string, quiet bool) error {
	mismatches, err := r.runVerificationRsyncOnce(ctx, bin, task, files, prefix)
	if err != nil {
		return err
	}
	if len(mismatches) > 0 {
		if !quiet {
			fmt.Fprintln(r.Stdout, Colorize(ColorYellow, fmt.Sprintf("%s verification repair: syncing %d mismatched file(s)", prefix, len(mismatches))))
		}
		if _, err := r.runRsync(ctx, bin, verificationRepairRsyncArgs(task.RsyncArgs), strings.NewReader(strings.Join(mismatches, "\n")+"\n"), prefix, quiet); err != nil {
			return fmt.Errorf("%s verification repair failed: %w", task.SourceFolder, err)
		}
		mismatches, err = r.runVerificationRsyncOnce(ctx, bin, task, mismatches, prefix)
		if err != nil {
			return err
		}
		if len(mismatches) > 0 {
			return fmt.Errorf("%s verification found %d mismatch(es) after repair", task.SourceFolder, len(mismatches))
		}
	}
	if !quiet {
		fmt.Fprintln(r.Stdout, Colorize(ColorGreen, fmt.Sprintf("%s verification passed", prefix)))
	}
	return nil
}

func (r Runner) runVerificationRsyncOnce(ctx context.Context, bin string, task Task, files []string, prefix string) ([]string, error) {
	args := verificationRsyncArgs(task.RsyncArgs, len(files) > 0)
	var stdin io.Reader
	if len(files) > 0 {
		stdin = strings.NewReader(strings.Join(files, "\n") + "\n")
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	prepareCommand(cmd)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	stopCancelWatcher := watchCommandCancel(ctx, cmd)
	defer stopCancelWatcher()

	var mismatches []string
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if isVerificationMismatch(line) {
			mismatches = append(mismatches, verificationMismatchPath(line))
			fmt.Fprintln(r.Stdout, Colorize(ColorRed, fmt.Sprintf("%s VERIFY MISMATCH: %s", prefix, line)))
			continue
		}
		if strings.HasPrefix(line, "rsync") {
			fmt.Fprintln(r.Stdout, Colorize(ColorRed, fmt.Sprintf("%s VERIFY ERROR ?: %s", prefix, line)))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, waitAfterScanError(ctx, cmd, err)
	}
	if err := waitCommand(ctx, cmd); err != nil {
		return nil, fmt.Errorf("%s verification rsync failed: %w", task.SourceFolder, err)
	}
	return compactNonEmptyStrings(mismatches), nil
}

func verificationRsyncArgs(args []string, filesFromStdin bool) []string {
	out := make([]string, 0, len(args)+3)
	insertAt := len(args)
	if len(args) >= 2 {
		insertAt = len(args) - 2
	}
	hasDryRun := false
	hasChecksum := false
	for _, arg := range args {
		if arg == "--dry-run" {
			hasDryRun = true
		}
		if arg == "--checksum" || arg == "-c" || strings.Contains(arg, "c") && strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			hasChecksum = true
		}
	}
	for i, arg := range args {
		if i == insertAt {
			if !hasDryRun {
				out = append(out, "--dry-run")
			}
			if !hasChecksum {
				out = append(out, "--checksum")
			}
			if filesFromStdin {
				out = append(out, "--files-from=-")
			}
		}
		if arg == "--delete" {
			continue
		}
		out = append(out, arg)
	}
	return out
}

func verificationRepairRsyncArgs(args []string) []string {
	out := make([]string, 0, len(args)+2)
	insertAt := len(args)
	if len(args) >= 2 {
		insertAt = len(args) - 2
	}
	hasChecksum := false
	for _, arg := range args {
		if arg == "--checksum" || arg == "-c" || strings.Contains(arg, "c") && strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			hasChecksum = true
		}
	}
	for i, arg := range args {
		if i == insertAt {
			if !hasChecksum {
				out = append(out, "--checksum")
			}
			out = append(out, "--files-from=-")
		}
		if arg == "--delete" || arg == "--dry-run" {
			continue
		}
		out = append(out, arg)
	}
	return out
}

func isVerificationMismatch(line string) bool {
	if len(line) < 2 {
		return false
	}
	switch line[0] {
	case '>', '<', 'c':
	default:
		return false
	}
	switch line[1] {
	case 'f', 'L', 'D', 'S':
		return true
	default:
		return false
	}
}

func verificationMismatchPath(line string) string {
	if len(line) > 12 && line[11] == ' ' {
		return line[12:]
	}
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		return strings.Join(fields[1:], " ")
	}
	return ""
}

func compactNonEmptyStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func auditFiles(task Task) ([]string, error) {
	selected := &auditHeap{}
	heap.Init(selected)
	salt := time.Now().UTC().Format("2006-01-02")
	err := filepath.WalkDir(task.SourceFolder, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if task.Kind == TaskRootFilesOnly && path != task.SourceFolder {
				return filepath.SkipDir
			}
			if path != task.SourceFolder && isExcludedPath(task.Location.Source, path, task.Location.ExcludePrefixes, task.Location.ExcludeStrings) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if isExcludedPath(task.Location.Source, path, task.Location.ExcludePrefixes, task.Location.ExcludeStrings) {
			return nil
		}
		rel, err := filepath.Rel(task.SourceFolder, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !matchesLocationPattern(task.Location.Pattern, rel) {
			return nil
		}
		item := auditItem{path: rel, score: auditScore(salt, task.SourceFolder, rel)}
		if selected.Len() < auditMaxFilesPerTask {
			heap.Push(selected, item)
			return nil
		}
		if item.score < (*selected)[0].score {
			heap.Pop(selected)
			heap.Push(selected, item)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, selected.Len())
	for i := len(out) - 1; i >= 0; i-- {
		out[i] = heap.Pop(selected).(auditItem).path
	}
	return out, nil
}

func matchesLocationPattern(pattern, rel string) bool {
	if pattern == "" || pattern == "**" {
		return true
	}
	if ok, _ := filepath.Match(pattern, filepath.Base(rel)); ok {
		return true
	}
	ok, _ := filepath.Match(filepath.ToSlash(pattern), rel)
	return ok
}

func auditScore(salt, root, rel string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(salt))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(filepath.ToSlash(root)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(rel))
	return h.Sum64()
}

type auditItem struct {
	path  string
	score uint64
}

type auditHeap []auditItem

func (h auditHeap) Len() int {
	return len(h)
}

func (h auditHeap) Less(i, j int) bool {
	return h[i].score > h[j].score
}

func (h auditHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *auditHeap) Push(x any) {
	*h = append(*h, x.(auditItem))
}

func (h *auditHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}
