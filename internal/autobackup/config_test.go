package autobackup

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigMissingFileMessage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	_, err := LoadConfig(path)
	var notFound ConfigNotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected ConfigNotFoundError, got %T %v", err, err)
	}
	for _, want := range []string{"config file not found", "--config", "AUTO_BACKUP_CONFIG", DefaultConfigName} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in error:\n%s", want, err)
		}
	}
}

func TestResolveConfigPathPrefersCurrentDirectory(t *testing.T) {
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatal(err)
		}
	})
	if err := os.WriteFile(DefaultConfigName, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := ResolveConfigPath("", filepath.Join(tmp, "dist", "linux-amd64", "autobackup"))
	if got != filepath.Join(".", DefaultConfigName) {
		t.Fatalf("got %q", got)
	}
}

func TestResolveConfigPathFallsBackToExecutableDirectory(t *testing.T) {
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatal(err)
		}
	})
	exe := filepath.Join(tmp, "dist", "linux-amd64", "autobackup")
	want := filepath.Join(tmp, "dist", "linux-amd64", DefaultConfigName)
	got := ResolveConfigPath("", exe)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestLoadConfigDefaultsAndFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	err := os.WriteFile(path, []byte(`{
		"destination": {"host": "host", "username": "pi", "base-path": "/backup", "identity-file": "/keys/id"},
		"quiet": true,
		"locations": [{"source": "/tmp/source", "destination": "dest", "parallel-rsync": false}]
	}`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Destination.BasePath != "/backup" {
		t.Fatalf("unexpected base path: %q", cfg.Destination.BasePath)
	}
	if cfg.Locations[0].ParallelRsync == nil || *cfg.Locations[0].ParallelRsync {
		t.Fatalf("parallel-rsync was not loaded as explicit false: %#v", cfg.Locations[0].ParallelRsync)
	}
	if cfg.Locations[0].Pattern != "**" {
		t.Fatalf("unexpected default pattern: %q", cfg.Locations[0].Pattern)
	}
	if cfg.Locations[0].Verification != string(VerifyAudit) {
		t.Fatalf("unexpected default verification: %q", cfg.Locations[0].Verification)
	}
	if !cfg.Quiet {
		t.Fatal("quiet was not loaded")
	}
}

func TestLoadConfigAcceptsParallelRsyncTrue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	err := os.WriteFile(path, []byte(`{
		"destination": {"host": "host", "username": "pi", "base-path": "/backup"},
		"locations": [{"source": "/tmp/source", "destination": "dest", "parallel-rsync": true}]
	}`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Locations[0].ParallelRsync == nil || !*cfg.Locations[0].ParallelRsync {
		t.Fatalf("parallel-rsync was not loaded as explicit true: %#v", cfg.Locations[0].ParallelRsync)
	}
}

func TestLoadConfigRequiresBasePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	err := os.WriteFile(path, []byte(`{
		"destination": {"host": "host", "username": "pi"},
		"locations": [{"source": "/tmp/source", "destination": "dest"}]
	}`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "destination.base-path") {
		t.Fatalf("expected base-path validation error, got %v", err)
	}
}

func TestLoadConfigRejectsInvalidVerification(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	err := os.WriteFile(path, []byte(`{
		"destination": {"host": "host", "username": "pi", "base-path": "/backup"},
		"locations": [{"source": "/tmp/source", "destination": "dest", "verification": "manifest"}]
	}`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "verification") {
		t.Fatalf("expected verification validation error, got %v", err)
	}
}
