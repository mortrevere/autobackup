package autobackup

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuildPlanDefaultJobs(t *testing.T) {
	oldLookPath := execLookPath
	execLookPath = func(file string) (string, error) {
		return file, nil
	}
	t.Cleanup(func() {
		execLookPath = oldLookPath
	})

	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	plan, err := BuildPlan(Config{
		Destination: Destination{Host: "host", Username: "pi", BasePath: "/backup"},
		Locations:   []Location{{Source: source, Destination: "dest"}},
	}, Options{GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Jobs != DefaultJobs {
		t.Fatalf("jobs got %d want %d", plan.Jobs, DefaultJobs)
	}
}

func TestBuildPlanQuietFromConfigOrOptions(t *testing.T) {
	oldLookPath := execLookPath
	execLookPath = func(file string) (string, error) {
		return file, nil
	}
	t.Cleanup(func() {
		execLookPath = oldLookPath
	})

	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	base := Config{
		Destination: Destination{Host: "host", Username: "pi", BasePath: "/backup"},
		Locations:   []Location{{Source: source, Destination: "dest"}},
	}
	plan, err := BuildPlan(base, Options{GOOS: "linux", GOARCH: "amd64", Quiet: true})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Quiet {
		t.Fatal("quiet option was not applied")
	}
	base.Quiet = true
	plan, err = BuildPlan(base, Options{GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Quiet {
		t.Fatal("quiet config was not applied")
	}
}

func TestBuildTaskRsyncArgs(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source dir")
	folder := filepath.Join(source, "sub folder")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}

	task, err := BuildTask(
		Destination{Host: "host", Username: "pi", BasePath: "/backup root", IdentityFile: "/keys/id key"},
		Location{Source: source, Destination: "dest folder", Pattern: "*.pdf", Delete: true, ExcludePrefixes: []string{"FL", "Microsoft VS Code", `nested\prefix`}, ExcludeStrings: []string{".venv", "node_modules"}},
		folder,
		Tools{Rsync: "rsync", SSH: "ssh"},
		Options{DryRun: true, GOOS: "linux"},
	)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(task.RsyncArgs, "\x00")
	for _, want := range []string{"--dry-run", "--delete", "--include=*.pdf", "--exclude=*", "--exclude=/FL/***", "--exclude=/Microsoft VS Code/***", "--exclude=/nested/prefix/***", "--exclude=*.venv*", "--exclude=*node_modules*", "-e"} {
		if !strings.Contains(args, want) {
			t.Fatalf("args missing %q: %#v", want, task.RsyncArgs)
		}
	}
	if !strings.Contains(task.RsyncArgs[len(task.RsyncArgs)-3], "'/keys/id key'") {
		t.Fatalf("identity file not quoted in ssh shell: %#v", task.RsyncArgs)
	}
	if task.RemoteFolder != "/backup root/dest folder/sub folder" {
		t.Fatalf("remote folder got %q", task.RemoteFolder)
	}
	if task.RsyncArgs[len(task.RsyncArgs)-1] != "pi@host:/backup root/dest folder/sub folder" {
		t.Fatalf("remote rsync target got %q", task.RsyncArgs[len(task.RsyncArgs)-1])
	}
	if task.MkdirArgs[len(task.MkdirArgs)-1] != "'/backup root/dest folder/sub folder'" {
		t.Fatalf("mkdir path was not shell-quoted: %#v", task.MkdirArgs)
	}
}

func TestBuildTaskRootFilesOnlyExcludesDirectories(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	task, err := BuildTask(
		Destination{Host: "host", Username: "pi", BasePath: "/backup"},
		Location{Source: source, Destination: "dest"},
		source,
		Tools{Rsync: "rsync", SSH: "ssh"},
		Options{GOOS: "linux", TaskKind: TaskRootFilesOnly},
	)
	if err != nil {
		t.Fatal(err)
	}
	if task.Kind != TaskRootFilesOnly {
		t.Fatalf("kind got %q want %q", task.Kind, TaskRootFilesOnly)
	}
	args := strings.Join(task.RsyncArgs, "\x00")
	if !strings.Contains(args, "--exclude=*/") {
		t.Fatalf("root-files task does not exclude directories: %#v", task.RsyncArgs)
	}
	if task.RemoteFolder != "/backup/dest" {
		t.Fatalf("remote folder got %q", task.RemoteFolder)
	}
}

func TestDiscoverFoldersHonorsExcludes(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	keep := filepath.Join(source, "keep")
	skip := filepath.Join(source, "skip")
	venv := filepath.Join(source, "project", ".venv", "lib")
	if err := os.MkdirAll(keep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(skip, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(venv, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := DiscoverFolders(Location{
		Source:          source,
		Destination:     "dest",
		Pattern:         "**",
		ExcludePrefixes: []string{"skip"},
		ExcludeStrings:  []string{".venv"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(got, []string{keep, filepath.Join(source, "project")}) {
		return
	}
	t.Fatalf("unexpected folders: %#v", got)
}

func TestDiscoverFoldersWithModeReportsHeuristicSplit(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "b"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, mode, err := DiscoverFoldersWithMode(Location{
		Source:      source,
		Destination: "dest",
		Pattern:     "**",
	})
	if err != nil {
		t.Fatal(err)
	}
	if mode != SplitHeuristicSplit {
		t.Fatalf("mode got %q want %q", mode, SplitHeuristicSplit)
	}
}

func TestDiscoverFoldersWithModeReportsHeuristicRootFilesAsSplit(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "root.txt"), []byte("root"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, mode, err := DiscoverFoldersWithMode(Location{
		Source:      source,
		Destination: "dest",
		Pattern:     "**",
	})
	if err != nil {
		t.Fatal(err)
	}
	if mode != SplitHeuristicSplit {
		t.Fatalf("mode got %q want %q", mode, SplitHeuristicSplit)
	}
}

func TestDiscoverFoldersWithModeIgnoresExplicitParallelRsync(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	parallel := false
	_, mode, err := DiscoverFoldersWithMode(Location{
		Source:        source,
		Destination:   "dest",
		Pattern:       "**",
		ParallelRsync: &parallel,
	})
	if err != nil {
		t.Fatal(err)
	}
	if mode != SplitExplicit {
		t.Fatalf("mode got %q want explicit", mode)
	}
}

func TestDiscoverFoldersIsolatesDeepTopFolder(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	deep := filepath.Join(source, "project", "a", "b", "c", "d", "e", "f")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := DiscoverFolders(Location{
		Source:      source,
		Destination: "dest",
		Pattern:     "**",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(source, "other"), filepath.Join(source, "project")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected folders: %#v", got)
	}
}

func TestDiscoverFoldersIsolatesVCSTopFolder(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	gitObjects := filepath.Join(source, "project", ".git", "objects")
	if err := os.MkdirAll(gitObjects, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := DiscoverFolders(Location{
		Source:      source,
		Destination: "dest",
		Pattern:     "**",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(source, "other"), filepath.Join(source, "project")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected folders: %#v", got)
	}
}

func TestDiscoverFoldersAddsRootTaskWhenRootHasFiles(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "root.txt"), []byte("root"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := DiscoverFolders(Location{
		Source:      source,
		Destination: "dest",
		Pattern:     "**",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{source, filepath.Join(source, "other"), filepath.Join(source, "project")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected folders: %#v", got)
	}
}

func TestDiscoverTasksClassifiesMixedTree(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	for _, path := range []string{
		filepath.Join(source, "normal"),
		filepath.Join(source, "repo", ".git", "objects"),
		filepath.Join(source, "deep", "a", "b", "c", "d", "e", "f"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "root.txt"), []byte("root"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := DiscoverTasks(Location{
		Source:      source,
		Destination: "dest",
		Pattern:     "**",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []discoveredTask{
		{Folder: source, Kind: TaskRootFilesOnly, SplitMode: SplitHeuristicSplit},
		{Folder: filepath.Join(source, "deep"), SplitMode: SplitHeuristicRecursive},
		{Folder: filepath.Join(source, "normal"), SplitMode: SplitHeuristicSplit},
		{Folder: filepath.Join(source, "repo"), SplitMode: SplitHeuristicRecursive},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected discovered tasks:\ngot  %#v\nwant %#v", got, want)
	}
}

func TestDiscoverFoldersNoTopLevelLimit(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	for i := 0; i < 70; i++ {
		if err := os.MkdirAll(filepath.Join(source, fmt.Sprintf("dir-%02d", i)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := DiscoverFolders(Location{
		Source:      source,
		Destination: "dest",
		Pattern:     "**",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 70 {
		t.Fatalf("folder count got %d want 70", len(got))
	}
}

func TestDiscoverFoldersExplicitFalseBypassesHeuristic(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, "project"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(source, "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	parallel := false
	got, err := DiscoverFolders(Location{
		Source:        source,
		Destination:   "dest",
		Pattern:       "**",
		ParallelRsync: &parallel,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{source}) {
		t.Fatalf("unexpected folders: %#v", got)
	}
}

func TestDiscoverFoldersExplicitTrueBypassesHeuristic(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	project := filepath.Join(source, "project")
	deep := filepath.Join(project, "a", "b", "c", "d", "e", "f")
	other := filepath.Join(source, "other")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	parallel := true
	got, err := DiscoverFolders(Location{
		Source:        source,
		Destination:   "dest",
		Pattern:       "**",
		ParallelRsync: &parallel,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		source,
		other,
		project,
		filepath.Join(project, "a"),
		filepath.Join(project, "a", "b"),
		filepath.Join(project, "a", "b", "c"),
		filepath.Join(project, "a", "b", "c", "d"),
		filepath.Join(project, "a", "b", "c", "d", "e"),
		deep,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected folders: %#v", got)
	}
}

func TestSSHArgsWithIdentityFile(t *testing.T) {
	got := SSHArgs(Destination{IdentityFile: "/keys/id"}, "linux")
	joined := strings.Join(got, "\x00")
	for _, want := range []string{"BatchMode=yes", "IdentitiesOnly=yes", "LogLevel=ERROR", "GlobalKnownHostsFile=/dev/null", "-i", "/keys/id"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %#v", want, got)
		}
	}
	wantTail := []string{"-i", "/keys/id"}
	if !reflect.DeepEqual(got[len(got)-2:], wantTail) {
		t.Fatalf("got %#v", got)
	}
}

func TestSSHArgsUsesWindowsNullDevice(t *testing.T) {
	got := strings.Join(SSHArgs(Destination{}, "windows"), "\x00")
	for _, want := range []string{"UserKnownHostsFile=NUL", "GlobalKnownHostsFile=NUL"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "/dev/null") {
		t.Fatalf("windows ssh args include Unix null device: %q", got)
	}
}
