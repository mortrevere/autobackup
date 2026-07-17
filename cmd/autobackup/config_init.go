package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"autobackup/internal/autobackup"
)

func configReference() string {
	return fmt.Sprintf(`

Configuration keys

destination.host: Remote SSH host.
destination.username: Remote SSH username.
destination.base-path: Required remote root under which location destinations are created.
destination.identity-file: Optional SSH private key path.
tools.rsync: Optional explicit rsync path. Empty uses bundled tools, then PATH.
tools.ssh: Optional explicit ssh path. Empty uses bundled tools, then PATH.
jobs: Maximum concurrent rsync jobs. Default: %d.
quiet: Only print heartbeats, final summaries, and errors.
windows-path-style: One of auto, native, msys, cygwin. Default: auto.
locations[].source: Local source path to back up.
locations[].destination: Remote folder name under destination.base-path.
locations[].parallel-rsync: Optional override. Omit for automatic planning.
locations[].pattern: Include pattern. Default: **.
locations[].verification: One of changed, audit, full. Default: audit.
locations[].exclude-prefixes: Source-relative prefixes to skip entirely.
locations[].exclude-strings: Source-relative path substrings to skip.
locations[].delete: Pass --delete to rsync, mirroring removals to the destination.

Run with -init to create a configuration file.
`, autobackup.DefaultJobs)
}

func runInit(in io.Reader, out io.Writer, path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}

	reader := bufio.NewReader(in)
	interactive, err := askYesNo(reader, out, "Configure interactively? Answer no to write a default valid config now. [Y/n]: ", true)
	if err != nil {
		return err
	}

	cfg := defaultInitConfig()
	if interactive {
		cfg, err = askConfig(reader, out)
		if err != nil {
			return err
		}
	}
	if err := writeConfig(absPath, cfg); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s\n", absPath)
	return nil
}

func defaultInitConfig() autobackup.Config {
	return autobackup.Config{
		Destination: autobackup.Destination{
			Host:         "backup.example.com",
			Username:     "backup",
			BasePath:     "/srv/backups/workstation",
			IdentityFile: "",
		},
		Tools: autobackup.ToolPaths{
			Rsync: "",
			SSH:   "",
		},
		Jobs:             autobackup.DefaultJobs,
		Quiet:            false,
		WindowsPathStyle: string(autobackup.PathAuto),
		Locations: []autobackup.Location{
			{
				Source:          ".",
				Destination:     "workstation",
				Pattern:         "**",
				Verification:    string(autobackup.VerifyAudit),
				ExcludePrefixes: []string{},
				ExcludeStrings:  []string{},
				Delete:          false,
			},
		},
	}
}

type initExamples struct {
	IdentityFile string
	Source       string
	Exclusion    string
}

func platformInitExamples(goos string) initExamples {
	if goos == "windows" {
		return initExamples{
			IdentityFile: `C:\Users\me\.ssh\id_ed25519`,
			Source:       `C:\Users\me\Documents`,
			Exclusion:    `AppData\Local\Temp`,
		}
	}
	return initExamples{
		IdentityFile: "/home/me/.ssh/id_ed25519",
		Source:       "/home/me/Documents",
		Exclusion:    ".venv",
	}
}

func askConfig(reader *bufio.Reader, out io.Writer) (autobackup.Config, error) {
	cfg := defaultInitConfig()
	examples := platformInitExamples(runtime.GOOS)
	var err error
	cfg.Destination.Host, err = askRequired(reader, out, "Destination host (example: backup.example.com): ")
	if err != nil {
		return autobackup.Config{}, err
	}
	cfg.Destination.Username, err = askRequired(reader, out, "Destination username (example: backup): ")
	if err != nil {
		return autobackup.Config{}, err
	}
	cfg.Destination.BasePath, err = askRequired(reader, out, "Destination base path on the remote host (example: /srv/backups/workstation): ")
	if err != nil {
		return autobackup.Config{}, err
	}
	cfg.Destination.IdentityFile, err = askOptional(reader, out, fmt.Sprintf("SSH key location on this machine (optional, example: %s): ", examples.IdentityFile))
	if err != nil {
		return autobackup.Config{}, err
	}

	loc := cfg.Locations[0]
	loc.Source, err = askRequired(reader, out, fmt.Sprintf("First source path to include on this machine (example: %s): ", examples.Source))
	if err != nil {
		return autobackup.Config{}, err
	}
	loc.Destination, err = askRequired(reader, out, "Remote destination folder for that source (example: documents): ")
	if err != nil {
		return autobackup.Config{}, err
	}
	loc.Pattern, err = askDefault(reader, out, "Include pattern (example: ** or *.pdf) [**]: ", "**")
	if err != nil {
		return autobackup.Config{}, err
	}
	exclusion, err := askOptional(reader, out, fmt.Sprintf("Possible exclusion, as path text to skip on this machine (optional, example: %s): ", examples.Exclusion))
	if err != nil {
		return autobackup.Config{}, err
	}
	if exclusion != "" {
		loc.ExcludeStrings = []string{exclusion}
	}
	sync, err := askSyncMode(reader, out)
	if err != nil {
		return autobackup.Config{}, err
	}
	loc.Delete = sync
	cfg.Locations = []autobackup.Location{loc}
	return cfg, nil
}

func askRequired(reader *bufio.Reader, out io.Writer, prompt string) (string, error) {
	for {
		value, err := askOptional(reader, out, prompt)
		if err != nil {
			return "", err
		}
		if value != "" {
			return value, nil
		}
		fmt.Fprintln(out, "Please enter a value.")
	}
}

func askDefault(reader *bufio.Reader, out io.Writer, prompt, fallback string) (string, error) {
	value, err := askOptional(reader, out, prompt)
	if err != nil {
		return "", err
	}
	if value == "" {
		return fallback, nil
	}
	return value, nil
}

func askOptional(reader *bufio.Reader, out io.Writer, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	value, err := reader.ReadString('\n')
	if err == io.EOF && value == "" {
		return "", err
	}
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func askYesNo(reader *bufio.Reader, out io.Writer, prompt string, fallback bool) (bool, error) {
	for {
		answer, err := askOptional(reader, out, prompt)
		if err != nil {
			return false, err
		}
		switch strings.ToLower(answer) {
		case "":
			return fallback, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(out, "Please answer yes or no.")
		}
	}
}

func askSyncMode(reader *bufio.Reader, out io.Writer) (bool, error) {
	for {
		answer, err := askDefault(reader, out, "Destination mode (example: append keeps old destination files, sync deletes removed source files) [append/sync, default append]: ", "append")
		if err != nil {
			return false, err
		}
		switch strings.ToLower(answer) {
		case "append", "a":
			return false, nil
		case "sync", "s":
			return true, nil
		default:
			fmt.Fprintln(out, "Please answer append or sync.")
		}
	}
}

func writeConfig(path string, cfg autobackup.Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("config already exists: %s", path)
		}
		return fmt.Errorf("create config: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(b); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
