package autobackup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const DefaultConfigName = "autobackup.config.json"

type Config struct {
	Destination      Destination `json:"destination"`
	Locations        []Location  `json:"locations"`
	Tools            ToolPaths   `json:"tools"`
	WindowsPathStyle string      `json:"windows-path-style"`
	Jobs             int         `json:"jobs"`
	Quiet            bool        `json:"quiet"`
}

type Destination struct {
	Host         string `json:"host"`
	Username     string `json:"username"`
	BasePath     string `json:"base-path"`
	IdentityFile string `json:"identity-file"`
}

type ToolPaths struct {
	Rsync string `json:"rsync"`
	SSH   string `json:"ssh"`
}

type Location struct {
	Source          string   `json:"source"`
	Destination     string   `json:"destination"`
	ParallelRsync   *bool    `json:"parallel-rsync,omitempty"`
	Pattern         string   `json:"pattern"`
	Verification    string   `json:"verification"`
	ExcludePrefixes []string `json:"exclude-prefixes"`
	ExcludeStrings  []string `json:"exclude-strings"`
	Delete          bool     `json:"delete"`
}

func ExecutableConfigPath(executable string) string {
	if executable == "" {
		return DefaultConfigName
	}
	p, err := filepath.Abs(executable)
	if err != nil {
		return DefaultConfigName
	}
	return filepath.Join(filepath.Dir(p), DefaultConfigName)
}

func DefaultConfigPath(executable string) string {
	cwdPath := filepath.Join(".", DefaultConfigName)
	if _, err := os.Stat(cwdPath); err == nil {
		return cwdPath
	}
	return ExecutableConfigPath(executable)
}

func ResolveConfigPath(flagPath, executable string) string {
	if flagPath != "" {
		return flagPath
	}
	if envPath := os.Getenv("AUTO_BACKUP_CONFIG"); envPath != "" {
		return envPath
	}
	return DefaultConfigPath(executable)
}

func LoadConfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Config{}, ConfigNotFoundError{Path: path}
		}
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	cfg.applyDefaults()
	return cfg, cfg.Validate()
}

type ConfigNotFoundError struct {
	Path string
}

func (e ConfigNotFoundError) Error() string {
	return fmt.Sprintf("config file not found: %s\n\nCreate a config file or point autobackup at one explicitly:\n  autobackup --config /path/to/autobackup.config.json\n\nWhen --config and AUTO_BACKUP_CONFIG are omitted, autobackup looks for %s in the current directory first, then beside the executable.", e.Path, DefaultConfigName)
}

func (c *Config) applyDefaults() {
	for i := range c.Locations {
		if c.Locations[i].Pattern == "" {
			c.Locations[i].Pattern = "**"
		}
		if c.Locations[i].Verification == "" {
			c.Locations[i].Verification = string(VerifyAudit)
		}
	}
}

func (c Config) Validate() error {
	if c.Destination.Host == "" {
		return errors.New("destination.host must be a non-empty string")
	}
	if c.Destination.Username == "" {
		return errors.New("destination.username must be a non-empty string")
	}
	if c.Destination.BasePath == "" {
		return errors.New("destination.base-path must be a non-empty string")
	}
	if len(c.Locations) == 0 {
		return errors.New("locations must contain at least one entry")
	}
	switch c.WindowsPathStyle {
	case "", "auto", "native", "msys", "cygwin":
	default:
		return errors.New("windows-path-style must be one of auto, native, msys, cygwin")
	}
	if c.Jobs < 0 {
		return errors.New("jobs must not be negative")
	}
	for i, loc := range c.Locations {
		if loc.Source == "" {
			return fmt.Errorf("locations[%d].source must be a non-empty string", i)
		}
		if loc.Destination == "" {
			return fmt.Errorf("locations[%d].destination must be a non-empty string", i)
		}
		switch VerificationMode(loc.Verification) {
		case VerifyChanged, VerifyAudit, VerifyFull:
		default:
			return fmt.Errorf("locations[%d].verification must be one of changed, audit, full", i)
		}
	}
	return nil
}
