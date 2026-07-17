package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"autobackup/internal/autobackup"
)

func TestRunInitWritesDefaultConfigWhenNotInteractive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "autobackup.config.json")
	var out strings.Builder

	if err := runInit(strings.NewReader("n\n"), &out, path); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), path) {
		t.Fatalf("output %q did not include config path %q", out.String(), path)
	}
	cfg, err := autobackup.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Destination.Host != "backup.example.com" {
		t.Fatalf("host got %q", cfg.Destination.Host)
	}
	if cfg.Jobs != autobackup.DefaultJobs {
		t.Fatalf("jobs got %d want %d", cfg.Jobs, autobackup.DefaultJobs)
	}
	if cfg.WindowsPathStyle != string(autobackup.PathAuto) {
		t.Fatalf("windows-path-style got %q", cfg.WindowsPathStyle)
	}
	if cfg.Locations[0].Verification != string(autobackup.VerifyAudit) {
		t.Fatalf("verification got %q", cfg.Locations[0].Verification)
	}
}

func TestRunInitWritesInteractiveConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "autobackup.config.json")
	input := strings.Join([]string{
		"y",
		"nas.local",
		"backup",
		"/srv/backups/laptop",
		"/home/me/.ssh/id_ed25519",
		"/home/me/Documents",
		"documents",
		"*.pdf",
		".cache",
		"sync",
		"",
	}, "\n")

	var out strings.Builder
	if err := runInit(strings.NewReader(input), &out, path); err != nil {
		t.Fatal(err)
	}
	examples := platformInitExamples(runtime.GOOS)
	for _, want := range []string{
		"backup.example.com",
		"base path on the remote host",
		examples.IdentityFile,
		examples.Source,
		"documents",
		"** or *.pdf",
		examples.Exclusion,
		"append keeps old destination files",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("interactive prompts missing %q:\n%s", want, out.String())
		}
	}
	cfg, err := autobackup.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Destination.Host != "nas.local" {
		t.Fatalf("host got %q", cfg.Destination.Host)
	}
	if cfg.Destination.IdentityFile != "/home/me/.ssh/id_ed25519" {
		t.Fatalf("identity-file got %q", cfg.Destination.IdentityFile)
	}
	loc := cfg.Locations[0]
	if loc.Source != "/home/me/Documents" || loc.Destination != "documents" {
		t.Fatalf("location got %#v", loc)
	}
	if loc.Pattern != "*.pdf" {
		t.Fatalf("pattern got %q", loc.Pattern)
	}
	if len(loc.ExcludeStrings) != 1 || loc.ExcludeStrings[0] != ".cache" {
		t.Fatalf("exclude strings got %#v", loc.ExcludeStrings)
	}
	if !loc.Delete {
		t.Fatal("delete should be true for sync mode")
	}
}

func TestWriteConfigDoesNotOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "autobackup.config.json")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := writeConfig(path, defaultInitConfig())
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already exists error, got %v", err)
	}
}
