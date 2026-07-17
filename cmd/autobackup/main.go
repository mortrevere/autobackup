package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"autobackup/internal/autobackup"
)

var version = "dev"

func main() {
	var configPath string
	var dryRun bool
	var jobs int
	var quiet bool
	var showVersion bool
	var windowsPathStyle string

	flag.StringVar(&configPath, "config", "", "path to autobackup JSON config")
	flag.BoolVar(&dryRun, "dry-run", false, "pass --dry-run to rsync")
	flag.IntVar(&jobs, "jobs", 0, fmt.Sprintf("maximum concurrent rsync jobs; defaults to config jobs or %d", autobackup.DefaultJobs))
	flag.BoolVar(&quiet, "quiet", false, "only print heartbeats, final summaries, and errors")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.StringVar(&windowsPathStyle, "windows-path-style", "", "override Windows source path style: auto, native, msys, cygwin")
	flag.Usage = func() {
		printUsage(flag.CommandLine.Output())
	}
	flag.Parse()

	if showVersion {
		fmt.Printf("autobackup %s %s/%s\n", version, runtime.GOOS, runtime.GOARCH)
		return
	}

	exe, _ := os.Executable()
	path := autobackup.ResolveConfigPath(configPath, exe)
	cfg, err := autobackup.LoadConfig(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, autobackup.Colorize(autobackup.ColorRed, fmt.Sprintf("[CONFIG ERROR] %v", err)))
		os.Exit(2)
	}
	if windowsPathStyle != "" {
		cfg.WindowsPathStyle = windowsPathStyle
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, autobackup.Colorize(autobackup.ColorRed, fmt.Sprintf("[CONFIG ERROR] %v", err)))
		os.Exit(2)
	}

	plan, err := autobackup.BuildPlan(cfg, autobackup.Options{
		DryRun:           dryRun,
		Jobs:             jobs,
		Executable:       exe,
		WindowsPathStyle: windowsPathStyle,
		Quiet:            quiet,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, autobackup.Colorize(autobackup.ColorRed, fmt.Sprintf("[PLAN ERROR] %v", err)))
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	result := autobackup.Runner{Stdout: os.Stdout, Stderr: os.Stderr}.Run(ctx, plan)
	if len(result.Failures) > 0 {
		for _, err := range result.Failures {
			fmt.Fprintln(os.Stderr, autobackup.Colorize(autobackup.ColorRed, fmt.Sprintf("[BACKUP ERROR] %v", err)))
		}
		os.Exit(1)
	}
}

func printUsage(out io.Writer) {
	fmt.Fprintf(out, "Usage of %s:\n", os.Args[0])
	flag.PrintDefaults()
	fmt.Fprint(out, configTemplate())
}

func configTemplate() string {
	return fmt.Sprintf(`

Configuration template

JSON does not support comments, so this template uses "_comment" fields that
can be deleted after editing.

{
  "_comment": "Top-level autobackup configuration.",
  "destination": {
    "_comment": "Remote SSH destination. base-path is required and is the remote root under which location destinations are created.",
    "host": "10.0.0.100",
    "username": "pi",
    "base-path": "/tmp/disk/backup",
    "identity-file": "C:\\home\\autobackup\\ssh\\id_rsa"
  },
  "tools": {
    "_comment": "Optional explicit tool paths. If omitted, autobackup looks beside the executable under tools/<os>-<arch>/, then PATH.",
    "rsync": "C:\\msys64\\usr\\bin\\rsync.exe",
    "ssh": "C:\\msys64\\usr\\bin\\ssh.exe"
  },
  "jobs": %d,
  "quiet": false,
  "_quiet-comment": "When true, only print periodic heartbeats, final summaries, and errors.",
  "windows-path-style": "auto",
  "_windows-path-style-comment": "One of auto, native, msys, cygwin. auto is usually right for MSYS2/Cygwin rsync.",
  "locations": [
    {
      "_comment": "One source-to-destination backup entry.",
      "source": "C:\\home",
      "destination": "host-name-or-folder-name",
      "_parallel-rsync-comment": "Optional override. Omit for automatic planning; false uses one root rsync task; true creates directory-level rsync tasks.",
      "pattern": "**",
      "_pattern-comment": "Defaults to **. Use patterns like *.pdf to include only matching files while keeping directories.",
      "verification": "audit",
      "_verification-comment": "One of changed, audit, full. Defaults to audit. changed keeps the normal rsync transfer behavior without a post-transfer checksum pass.",
      "exclude-prefixes": [
        "Downloads",
        "Microsoft VS Code"
      ],
      "_exclude-prefixes-comment": "Source-relative prefixes to skip entirely.",
      "exclude-strings": [
        ".venv"
      ],
      "_exclude-strings-comment": "Skip any source-relative path containing these strings.",
      "delete": false,
      "_delete-comment": "When true, pass --delete to rsync for this location."
    }
  ]
}
`, autobackup.DefaultJobs)
}
