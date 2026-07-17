package autobackup

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunnerDryRunSkipsMkdirAndCountsChanges(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake commands are for Unix test hosts")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "ssh.log")
	sshPath := filepath.Join(dir, "ssh")
	rsyncPath := filepath.Join(dir, "rsync")
	if err := os.WriteFile(sshPath, []byte("#!/usr/bin/env bash\nprintf '%s\\n' \"$*\" >> "+shellQuote(logPath)+"\nprintf '/tmp/disk 1G 2M 998M 1%% /tmp/disk\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rsyncPath, []byte("#!/usr/bin/env bash\nprintf '<f+++++++++ file.txt\\n'\nprintf 'sent 1 bytes received 2 bytes\\n'\nprintf 'total size is 3 speedup 1.00\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	plan := Plan{
		Config: Config{
			Destination: Destination{Host: "host", Username: "pi", BasePath: "/tmp/disk/backup"},
		},
		Tools: Tools{Rsync: rsyncPath, SSH: sshPath},
		Tasks: []Task{{
			LocationName: "/source",
			SourceFolder: "/source",
			RemoteFolder: "/tmp/disk/backup/dest",
			RsyncArgs:    []string{"--dry-run", "/source/", "pi@host:/tmp/disk/backup/dest"},
			MkdirArgs:    []string{"pi@host", "mkdir", "-p", "/tmp/disk/backup/dest"},
		}},
		Jobs:   1,
		DryRun: true,
	}
	var out bytes.Buffer
	result := Runner{Stdout: &out}.Run(context.Background(), plan)
	if len(result.Failures) != 0 {
		t.Fatalf("unexpected failures: %#v", result.Failures)
	}
	if result.ChangedByLocation["/source"] != 1 {
		t.Fatalf("changed count = %d", result.ChangedByLocation["/source"])
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logBytes), "mkdir") {
		t.Fatalf("dry-run executed mkdir: %s", logBytes)
	}
	if !strings.Contains(out.String(), "dry-run: skipping remote mkdir") {
		t.Fatalf("missing dry-run mkdir message: %s", out.String())
	}
}

func TestCancelStopsRsyncProcessTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix process-group test")
	}
	dir := t.TempDir()
	childPidPath := filepath.Join(dir, "child.pid")
	rsyncPath := filepath.Join(dir, "rsync")
	if err := os.WriteFile(rsyncPath, []byte("#!/usr/bin/env bash\n(sleep 60) &\nprintf '%s\\n' \"$!\" > "+shellQuote(childPidPath)+"\nwait\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := (Runner{Stdout: ioDiscard{}}).runRsync(ctx, rsyncPath, nil, nil, "[test]:", false)
		done <- err
	}()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(childPidPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child process did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("rsync process did not stop after cancellation")
	}
	pidBytes, err := os.ReadFile(childPidPath)
	if err != nil {
		t.Fatal(err)
	}
	pid := strings.TrimSpace(string(pidBytes))
	if exec.Command("kill", "-0", pid).Run() == nil {
		t.Fatalf("child process %s still exists", pid)
	}
}

func TestRunRsyncPrintsFinalHundredAfterSuccessfulProgress(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake commands are for Unix test hosts")
	}
	dir := t.TempDir()
	rsyncPath := filepath.Join(dir, "rsync")
	if err := os.WriteFile(rsyncPath, []byte("#!/usr/bin/env bash\nprintf '        0   0%%    0.00kB/s    0:00:00\\n'\nprintf 'sent 1 bytes received 2 bytes\\n'\nprintf 'total size is 3 speedup 1.00\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	_, err := (Runner{Stdout: &out}).runRsync(context.Background(), rsyncPath, nil, nil, "[test]:", false)
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "[test]: 0%") || !strings.Contains(got, "[test]: 100%") {
		t.Fatalf("missing progress markers: %s", got)
	}
}

func TestRunRsyncDoesNotDuplicateHundredPercent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake commands are for Unix test hosts")
	}
	dir := t.TempDir()
	rsyncPath := filepath.Join(dir, "rsync")
	if err := os.WriteFile(rsyncPath, []byte("#!/usr/bin/env bash\nprintf '        3 100%%    0.00kB/s    0:00:00\\n'\nprintf 'sent 1 bytes received 2 bytes\\n'\nprintf 'total size is 3 speedup 1.00\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	_, err := (Runner{Stdout: &out}).runRsync(context.Background(), rsyncPath, nil, nil, "[test]:", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "[test]: 100%") != 1 {
		t.Fatalf("unexpected 100%% output: %s", out.String())
	}
}

func TestRunRsyncQuietSuppressesNormalOutputButKeepsErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake commands are for Unix test hosts")
	}
	dir := t.TempDir()
	rsyncPath := filepath.Join(dir, "rsync")
	if err := os.WriteFile(rsyncPath, []byte("#!/usr/bin/env bash\nprintf '<f+++++++++ file.txt\\n'\nprintf '        0   0%%    0.00kB/s    0:00:00\\n'\nprintf 'rsync warning: example\\n'\nprintf 'sent 1 bytes received 2 bytes\\n'\nprintf 'total size is 3 speedup 1.00\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	changed, err := (Runner{Stdout: &out}).runRsync(context.Background(), rsyncPath, nil, nil, "[test]:", true)
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if changed != 1 {
		t.Fatalf("changed got %d want 1", changed)
	}
	if strings.Contains(got, "<f+++++++++") || strings.Contains(got, "0%") || strings.Contains(got, "sent 1 bytes") {
		t.Fatalf("quiet output included normal logs: %s", got)
	}
	if !strings.Contains(got, "ERROR ? : rsync warning: example") {
		t.Fatalf("quiet output suppressed rsync error: %s", got)
	}
}

func TestJobHeartbeatReportsActiveAndRemainingTasks(t *testing.T) {
	oldInterval := jobHeartbeatInterval
	jobHeartbeatInterval = 10 * time.Millisecond
	t.Cleanup(func() {
		jobHeartbeatInterval = oldInterval
	})

	var active atomic.Int64
	var remaining atomic.Int64
	active.Store(2)
	remaining.Store(5)

	out := &safeBuffer{}
	stop := (Runner{Stdout: out}).startJobHeartbeat(context.Background(), &active, &remaining)
	deadline := time.Now().Add(500 * time.Millisecond)
	for !strings.Contains(out.String(), "still running: 2 rsync jobs, 5 enqueued") {
		if time.Now().After(deadline) {
			stop()
			t.Fatalf("heartbeat did not print status: %s", out.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
	stop()
}

type safeBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
