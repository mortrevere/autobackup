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
	var initConfig bool
	var windowsPathStyle string

	flag.StringVar(&configPath, "config", "", "path to autobackup JSON config")
	flag.BoolVar(&dryRun, "dry-run", false, "pass --dry-run to rsync")
	flag.IntVar(&jobs, "jobs", 0, fmt.Sprintf("maximum concurrent rsync jobs; defaults to config jobs or %d", autobackup.DefaultJobs))
	flag.BoolVar(&quiet, "quiet", false, "only print heartbeats, final summaries, and errors")
	flag.BoolVar(&initConfig, "init", false, "create an autobackup JSON config")
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
	if initConfig {
		if err := runInit(os.Stdin, os.Stdout, path); err != nil {
			fmt.Fprintln(os.Stderr, autobackup.Colorize(autobackup.ColorRed, fmt.Sprintf("[INIT ERROR] %v", err)))
			os.Exit(2)
		}
		return
	}

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
	fmt.Fprint(out, configReference())
}
