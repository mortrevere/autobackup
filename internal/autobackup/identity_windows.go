//go:build windows

package autobackup

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

func PrepareIdentityFile(ctx context.Context, path string, out io.Writer) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("identity-file %q is not accessible: %w", path, err)
	}
	user := windowsAccountName()
	if user == "" {
		return fmt.Errorf("cannot determine current Windows user for identity-file ACL repair")
	}
	commands := [][]string{
		{"icacls", path, "/inheritance:r"},
		{"icacls", path, "/grant:r", user + ":F"},
		{"icacls", path, "/remove:g", "*S-1-5-32-545", "*S-1-5-11", "*S-1-1-0"},
	}
	for _, args := range commands {
		if out != nil {
			fmt.Fprintf(out, "Checking identity-file permissions: %s\n", strings.Join(args, " "))
		}
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		if out != nil {
			cmd.Stdout = out
			cmd.Stderr = out
		}
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to repair permissions on identity-file %q: %w\nRun manually: icacls %q /inheritance:r /grant:r \"%s:F\" /remove:g *S-1-5-32-545 *S-1-5-11 *S-1-1-0", path, err, path, user)
		}
	}
	return nil
}

func windowsAccountName() string {
	user := os.Getenv("USERNAME")
	if user == "" {
		return ""
	}
	if domain := os.Getenv("USERDOMAIN"); domain != "" {
		return domain + `\` + user
	}
	return user
}
