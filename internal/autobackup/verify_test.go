package autobackup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerificationRsyncArgsAddsChecksumDryRunAndFilesFrom(t *testing.T) {
	args := verificationRsyncArgs([]string{
		"-avh",
		"-i",
		"--delete",
		"-e",
		"ssh",
		"/source/",
		"pi@host:/backup/dest",
	}, true)
	joined := strings.Join(args, "\x00")
	for _, want := range []string{"--dry-run", "--checksum", "--files-from=-"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %#v", want, args)
		}
	}
	if strings.Contains(joined, "--delete") {
		t.Fatalf("verification args should not include --delete: %#v", args)
	}
	if args[len(args)-2] != "/source/" || args[len(args)-1] != "pi@host:/backup/dest" {
		t.Fatalf("source and destination moved: %#v", args)
	}
}

func TestVerificationRepairRsyncArgsAddsChecksumAndFilesFrom(t *testing.T) {
	args := verificationRepairRsyncArgs([]string{
		"-avh",
		"-i",
		"--dry-run",
		"--delete",
		"/source/",
		"pi@host:/backup/dest",
	})
	joined := strings.Join(args, "\x00")
	for _, want := range []string{"--checksum", "--files-from=-"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %#v", want, args)
		}
	}
	if strings.Contains(joined, "--delete") {
		t.Fatalf("repair args should not include --delete: %#v", args)
	}
	if strings.Contains(joined, "--dry-run") {
		t.Fatalf("repair args should not include --dry-run: %#v", args)
	}
	if args[len(args)-2] != "/source/" || args[len(args)-1] != "pi@host:/backup/dest" {
		t.Fatalf("source and destination moved: %#v", args)
	}
}

func TestVerificationMismatchPath(t *testing.T) {
	got := verificationMismatchPath("<fcs....... build/media/products/clip/Clip FP.PNG")
	if got != "build/media/products/clip/Clip FP.PNG" {
		t.Fatalf("path got %q", got)
	}
}

func TestAuditFilesHonorsExcludesAndPattern(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{
		filepath.Join(root, "keep", "a.wav"),
		filepath.Join(root, "keep", "b.txt"),
		filepath.Join(root, "skip", "c.wav"),
		filepath.Join(root, "project", ".venv", "d.wav"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := auditFiles(Task{
		SourceFolder: root,
		Location: Location{
			Source:          root,
			Pattern:         "*.wav",
			ExcludePrefixes: []string{"skip"},
			ExcludeStrings:  []string{".venv"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "keep/a.wav" {
		t.Fatalf("unexpected audit files: %#v", files)
	}
}

func TestAuditFilesRootFilesOnlySkipsSubdirectories(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{
		filepath.Join(root, "root.wav"),
		filepath.Join(root, "nested", "child.wav"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := auditFiles(Task{
		SourceFolder: root,
		Kind:         TaskRootFilesOnly,
		Location: Location{
			Source:  root,
			Pattern: "**",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "root.wav" {
		t.Fatalf("unexpected audit files: %#v", files)
	}
}
