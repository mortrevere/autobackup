package autobackup

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Options struct {
	DryRun           bool
	Jobs             int
	Executable       string
	GOOS             string
	GOARCH           string
	WindowsPathStyle string
	Quiet            bool
	TaskKind         TaskKind
}

type Tools struct {
	Rsync string
	SSH   string
}

type Task struct {
	LocationName string
	SourceFolder string
	RemoteFolder string
	Location     Location
	Kind         TaskKind
	SplitMode    SplitMode
	RsyncArgs    []string
	MkdirArgs    []string
}

type TaskKind string

const (
	TaskRecursive     TaskKind = ""
	TaskRootFilesOnly TaskKind = "root-files"
)

type SplitMode string

const (
	SplitExplicit           SplitMode = ""
	SplitHeuristicRecursive SplitMode = "recursive-rsync"
	SplitHeuristicSplit     SplitMode = "split-rsync"
)

type Plan struct {
	Config Config
	Tools  Tools
	Tasks  []Task
	Jobs   int
	DryRun bool
	Quiet  bool
}

const DefaultJobs = 16
const autoParallelMaxDepth = 6

type discoveredTask struct {
	Folder    string
	Kind      TaskKind
	SplitMode SplitMode
}

func BuildPlan(cfg Config, opts Options) (Plan, error) {
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return Plan{}, err
	}

	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := opts.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	exe := opts.Executable
	if exe == "" {
		exe, _ = os.Executable()
	}
	rsync, err := ResolveTool(cfg.Tools.Rsync, "rsync", exe, goos, goarch)
	if err != nil {
		return Plan{}, err
	}
	ssh, err := ResolveTool(cfg.Tools.SSH, "ssh", exe, goos, goarch)
	if err != nil {
		return Plan{}, err
	}

	jobs := opts.Jobs
	if jobs == 0 {
		jobs = cfg.Jobs
	}
	if jobs == 0 {
		jobs = DefaultJobs
	}
	pathStyle := opts.WindowsPathStyle
	if pathStyle == "" {
		pathStyle = cfg.WindowsPathStyle
	}
	if pathStyle == "" {
		pathStyle = string(PathAuto)
	}

	tools := Tools{Rsync: rsync, SSH: ssh}
	var tasks []Task
	for _, loc := range cfg.Locations {
		discovered, err := DiscoverTasks(loc)
		if err != nil {
			return Plan{}, fmt.Errorf("discover folders for %q: %w", loc.Source, err)
		}
		for _, discoveredTask := range discovered {
			task, err := BuildTask(cfg.Destination, loc, discoveredTask.Folder, tools, Options{
				DryRun:           opts.DryRun,
				GOOS:             goos,
				WindowsPathStyle: pathStyle,
				TaskKind:         discoveredTask.Kind,
			})
			if err != nil {
				return Plan{}, err
			}
			task.SplitMode = discoveredTask.SplitMode
			tasks = append(tasks, task)
		}
	}
	return Plan{Config: cfg, Tools: tools, Tasks: tasks, Jobs: jobs, DryRun: opts.DryRun, Quiet: cfg.Quiet || opts.Quiet}, nil
}

func DiscoverFolders(loc Location) ([]string, error) {
	tasks, err := DiscoverTasks(loc)
	if err != nil {
		return nil, err
	}
	folders := make([]string, 0, len(tasks))
	for _, task := range tasks {
		folders = append(folders, task.Folder)
	}
	return folders, nil
}

func DiscoverFoldersWithMode(loc Location) ([]string, SplitMode, error) {
	tasks, err := DiscoverTasks(loc)
	if err != nil {
		return nil, SplitExplicit, err
	}
	folders := make([]string, 0, len(tasks))
	mode := aggregateSplitMode(tasks)
	for _, task := range tasks {
		folders = append(folders, task.Folder)
	}
	return folders, mode, nil
}

func DiscoverTasks(loc Location) ([]discoveredTask, error) {
	pattern := loc.Pattern
	if pattern == "" {
		pattern = "**"
	}
	if pattern == "**" {
		if loc.ParallelRsync != nil {
			if *loc.ParallelRsync {
				folders, err := discoverAllFolders(loc)
				return discoveredRecursiveTasks(folders, SplitExplicit), err
			}
			return []discoveredTask{{Folder: loc.Source, SplitMode: SplitExplicit}}, nil
		}
		return discoverWholeTreeFolders(loc)
	}
	var folders []string
	matches, err := filepath.Glob(filepath.Join(loc.Source, pattern))
	if err != nil {
		return nil, err
	}
	for _, match := range matches {
		info, err := os.Stat(match)
		if err == nil && info.IsDir() && !isExcludedPath(loc.Source, match, loc.ExcludePrefixes, loc.ExcludeStrings) {
			folders = append(folders, match)
		}
	}
	folders = append(folders, loc.Source)
	return discoveredRecursiveTasks(folders, SplitExplicit), nil
}

func aggregateSplitMode(tasks []discoveredTask) SplitMode {
	mode := SplitExplicit
	for _, task := range tasks {
		if task.SplitMode == SplitExplicit {
			continue
		}
		if mode == SplitExplicit {
			mode = task.SplitMode
			continue
		}
		if mode != task.SplitMode {
			return SplitHeuristicSplit
		}
	}
	return mode
}

func discoveredRecursiveTasks(folders []string, mode SplitMode) []discoveredTask {
	out := make([]discoveredTask, 0, len(folders))
	for _, folder := range folders {
		out = append(out, discoveredTask{Folder: folder, SplitMode: mode})
	}
	return out
}

func discoverAllFolders(loc Location) ([]string, error) {
	var folders []string
	err := filepath.WalkDir(loc.Source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != loc.Source && isExcludedPath(loc.Source, path, loc.ExcludePrefixes, loc.ExcludeStrings) {
			return filepath.SkipDir
		}
		folders = append(folders, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return folders, nil
}

func discoverWholeTreeFolders(loc Location) ([]discoveredTask, error) {
	entries, err := os.ReadDir(loc.Source)
	if err != nil {
		return nil, err
	}
	var out []discoveredTask
	var folders []os.DirEntry
	rootFiles := false
	for _, entry := range entries {
		path := filepath.Join(loc.Source, entry.Name())
		if entry.IsDir() {
			if !isExcludedPath(loc.Source, path, loc.ExcludePrefixes, loc.ExcludeStrings) {
				folders = append(folders, entry)
			}
			continue
		}
		rootFiles = true
	}
	if rootFiles {
		out = append(out, discoveredTask{Folder: loc.Source, Kind: TaskRootFilesOnly, SplitMode: SplitHeuristicSplit})
	}
	for _, entry := range folders {
		folder := filepath.Join(loc.Source, entry.Name())
		recursive, err := needsRecursiveTopFolder(loc, folder)
		if err != nil {
			return nil, err
		}
		if recursive {
			out = append(out, discoveredTask{Folder: folder, SplitMode: SplitHeuristicRecursive})
			continue
		}
		out = append(out, discoveredTask{Folder: folder, SplitMode: SplitHeuristicSplit})
	}
	if len(out) == 0 {
		return []discoveredTask{{Folder: loc.Source, SplitMode: SplitHeuristicRecursive}}, nil
	}
	return out, nil
}

func needsRecursiveTopFolder(loc Location, folder string) (bool, error) {
	recursive := false
	err := filepath.WalkDir(folder, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != folder && isExcludedPath(loc.Source, path, loc.ExcludePrefixes, loc.ExcludeStrings) {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(loc.Source, path)
		if err != nil {
			return err
		}
		if isVCSPath(rel) || pathDepth(rel) > autoParallelMaxDepth {
			recursive = true
			return filepath.SkipAll
		}
		return nil
	})
	return recursive, err
}

func pathDepth(rel string) int {
	if rel == "." || rel == "" {
		return 0
	}
	rel = filepath.ToSlash(rel)
	return strings.Count(strings.Trim(rel, "/"), "/") + 1
}

func isVCSPath(rel string) bool {
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		switch part {
		case ".git", ".hg", ".svn":
			return true
		}
	}
	return false
}

func isExcludedPath(root, path string, prefixes []string, substrings []string) bool {
	for _, prefix := range prefixes {
		prefix = normalizePathPrefix(prefix)
		if prefix == "" {
			continue
		}
		excluded := filepath.Join(root, filepath.FromSlash(prefix))
		if path == excluded || strings.HasPrefix(path, excluded+string(os.PathSeparator)) {
			return true
		}
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	rel = filepath.ToSlash(rel)
	for _, substring := range substrings {
		substring = normalizePathSubstring(substring)
		if substring != "" && strings.Contains(rel, substring) {
			return true
		}
	}
	return false
}

func BuildTask(dest Destination, loc Location, folder string, tools Tools, opts Options) (Task, error) {
	baseDir, err := filepath.Rel(loc.Source, folder)
	if err != nil {
		return Task{}, err
	}
	pathStyle := WindowsPathStyle(opts.WindowsPathStyle)
	source := ToRsyncSourcePath(folder, opts.GOOS, pathStyle, tools.Rsync)
	source = strings.TrimRight(source, "/\\") + "/"
	remoteRel := JoinRemote(loc.Destination, filepath.ToSlash(baseDir))
	remoteFolder := JoinRemote(dest.BasePath, remoteRel)
	remoteTarget := fmt.Sprintf("%s@%s:%s", dest.Username, dest.Host, remoteFolder)
	sshArgs := SSHArgs(dest, opts.GOOS)

	rsyncArgs := []string{
		"-avh",
		"-i",
		"--modify-window=3700",
		"--protect-args",
		"--no-perms",
		"--no-group",
		"--no-owner",
		"--no-compress",
		"--info=progress2",
	}
	if opts.DryRun {
		rsyncArgs = append(rsyncArgs, "--dry-run")
	}
	if loc.Delete {
		rsyncArgs = append(rsyncArgs, "--delete")
	}
	if loc.Pattern != "" && loc.Pattern != "**" {
		rsyncArgs = append(rsyncArgs, "--include="+loc.Pattern, "--include=*/", "--exclude=*")
	}
	if opts.TaskKind == TaskRootFilesOnly {
		rsyncArgs = append(rsyncArgs, "--exclude=*/")
	}
	for _, exclude := range rsyncExcludeArgs(loc.ExcludePrefixes) {
		rsyncArgs = append(rsyncArgs, exclude...)
	}
	for _, exclude := range rsyncExcludeStringArgs(loc.ExcludeStrings) {
		rsyncArgs = append(rsyncArgs, exclude...)
	}
	rsyncArgs = append(rsyncArgs, "-e", shellJoin(append([]string{tools.SSH}, sshArgs...)), source, remoteTarget)

	mkdirArgs := append(SSHArgs(dest, opts.GOOS), fmt.Sprintf("%s@%s", dest.Username, dest.Host), "mkdir", "-p", shellQuote(remoteFolder))
	return Task{
		LocationName: loc.Source,
		SourceFolder: folder,
		RemoteFolder: remoteFolder,
		Location:     loc,
		Kind:         opts.TaskKind,
		RsyncArgs:    rsyncArgs,
		MkdirArgs:    mkdirArgs,
	}, nil
}

func rsyncExcludeStringArgs(substrings []string) [][]string {
	out := make([][]string, 0, len(substrings))
	for _, substring := range substrings {
		substring = normalizePathSubstring(substring)
		if substring == "" {
			continue
		}
		out = append(out, []string{"--exclude=*" + substring + "*"})
	}
	return out
}

func rsyncExcludeArgs(prefixes []string) [][]string {
	out := make([][]string, 0, len(prefixes)*2)
	for _, prefix := range prefixes {
		prefix = normalizePathPrefix(prefix)
		if prefix == "" {
			continue
		}
		out = append(out,
			[]string{"--exclude=/" + prefix},
			[]string{"--exclude=/" + prefix + "/***"},
		)
	}
	return out
}

func normalizePathPrefix(prefix string) string {
	prefix = strings.Trim(strings.ReplaceAll(filepath.ToSlash(prefix), "\\", "/"), "/")
	if prefix == "." {
		return ""
	}
	return prefix
}

func normalizePathSubstring(substring string) string {
	return strings.TrimSpace(strings.ReplaceAll(filepath.ToSlash(substring), "\\", "/"))
}

func SSHArgs(dest Destination, goos string) []string {
	if goos == "" {
		goos = runtime.GOOS
	}
	nullDevice := "/dev/null"
	if goos == "windows" {
		nullDevice = "NUL"
	}
	args := []string{
		"-o", "UserKnownHostsFile=" + nullDevice,
		"-o", "GlobalKnownHostsFile=" + nullDevice,
		"-o", "StrictHostKeyChecking=no",
		"-o", "LogLevel=ERROR",
		"-o", "BatchMode=yes",
	}
	if dest.IdentityFile != "" {
		args = append(args, "-o", "IdentitiesOnly=yes", "-i", dest.IdentityFile)
	}
	return args
}

func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, func(r rune) bool {
		return !(r == '/' || r == '.' || r == '-' || r == '_' || r == ':' || r == '=' || r == '\\' || r == '+' || r == ',' || r == '@' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z')
	}) == -1 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
