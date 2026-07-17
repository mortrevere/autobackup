package autobackup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestConvertWindowsPathStyles(t *testing.T) {
	tests := []struct {
		name  string
		style WindowsPathStyle
		want  string
	}{
		{"native", PathNative, `C:\Users\Leo\Docs`},
		{"msys", PathMSYS, "/c/Users/Leo/Docs"},
		{"cygwin", PathCygwin, "/cygdrive/c/Users/Leo/Docs"},
		{"auto", PathAuto, "/c/Users/Leo/Docs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertWindowsPath(`C:\Users\Leo\Docs`, tt.style, "rsync.exe")
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestJoinRemote(t *testing.T) {
	got := JoinRemote("/tmp/disk/backup", "dest", ".")
	if got != "/tmp/disk/backup/dest" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveToolAcceptsBundledWindowsToolWithoutExecutableBit(t *testing.T) {
	oldLookPath := execLookPath
	execLookPath = func(file string) (string, error) {
		return "", errors.New("not found")
	}
	t.Cleanup(func() {
		execLookPath = oldLookPath
	})

	root := t.TempDir()
	exe := filepath.Join(root, "autobackup.exe")
	tool := filepath.Join(root, "tools", "windows-amd64", "rsync.exe")
	if err := os.MkdirAll(filepath.Dir(tool), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tool, []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveTool("", "rsync", exe, "windows", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if got != tool {
		t.Fatalf("got %q want %q", got, tool)
	}
}

func TestResolveToolRejectsBundledUnixToolWithoutExecutableBit(t *testing.T) {
	oldLookPath := execLookPath
	execLookPath = func(file string) (string, error) {
		return "", errors.New("not found")
	}
	t.Cleanup(func() {
		execLookPath = oldLookPath
	})

	root := t.TempDir()
	exe := filepath.Join(root, "autobackup")
	tool := filepath.Join(root, "tools", "linux-amd64", "rsync")
	if err := os.MkdirAll(filepath.Dir(tool), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tool, []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := ResolveTool("", "rsync", exe, "linux", "amd64"); err == nil {
		t.Fatal("ResolveTool accepted a non-executable Unix tool")
	}
}
