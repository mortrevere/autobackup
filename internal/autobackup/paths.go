package autobackup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type WindowsPathStyle string

const (
	PathAuto   WindowsPathStyle = "auto"
	PathNative WindowsPathStyle = "native"
	PathMSYS   WindowsPathStyle = "msys"
	PathCygwin WindowsPathStyle = "cygwin"
)

func PlatformTag(goos, goarch string) string {
	return goos + "-" + goarch
}

func ToolName(name, goos string) string {
	if goos == "windows" && !strings.HasSuffix(strings.ToLower(name), ".exe") {
		return name + ".exe"
	}
	return name
}

func ResolveTool(configured, name, exePath, goos, goarch string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	toolName := ToolName(name, goos)
	if exePath != "" {
		if abs, err := filepath.Abs(exePath); err == nil {
			candidate := filepath.Join(filepath.Dir(abs), "tools", PlatformTag(goos, goarch), toolName)
			if isExecutable(candidate, goos) {
				return candidate, nil
			}
		}
	}
	if p, err := execLookPath(toolName); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("%s not found beside executable or on PATH", toolName)
}

var execLookPath = func(file string) (string, error) {
	return exec.LookPath(file)
}

func isExecutable(path, goos string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if goos == "windows" {
		return true
	}
	return info.Mode()&0111 != 0
}

func ConvertWindowsPath(path string, style WindowsPathStyle, bundledTool string) string {
	if style == "" || style == PathAuto {
		style = detectWindowsPathStyle(bundledTool)
	}
	if len(path) < 3 || path[1] != ':' || (path[2] != '\\' && path[2] != '/') {
		return filepath.ToSlash(path)
	}
	drive := strings.ToLower(path[:1])
	rest := strings.ReplaceAll(path[2:], "\\", "/")
	rest = strings.TrimPrefix(rest, "/")
	switch style {
	case PathNative:
		return path
	case PathCygwin:
		return "/cygdrive/" + drive + "/" + rest
	default:
		return "/" + drive + "/" + rest
	}
}

func detectWindowsPathStyle(tool string) WindowsPathStyle {
	lower := strings.ToLower(tool)
	switch {
	case strings.Contains(lower, "cygwin"):
		return PathCygwin
	case strings.Contains(lower, "msys"), strings.Contains(lower, "mingw"):
		return PathMSYS
	default:
		return PathMSYS
	}
}

func ToRsyncSourcePath(path string, goos string, style WindowsPathStyle, rsyncPath string) string {
	if goos == "windows" || looksLikeWindowsAbs(path) {
		return ConvertWindowsPath(path, style, rsyncPath)
	}
	return strings.ReplaceAll(filepath.ToSlash(path), "\\", "/")
}

func looksLikeWindowsAbs(path string) bool {
	return len(path) >= 3 && path[1] == ':' && (path[2] == '\\' || path[2] == '/')
}

func JoinRemote(parts ...string) string {
	absolute := len(parts) > 0 && strings.HasPrefix(parts[0], "/")
	var out []string
	for _, p := range parts {
		p = strings.Trim(p, "/")
		if p != "" && p != "." {
			out = append(out, p)
		}
	}
	joined := strings.Join(out, "/")
	if absolute {
		return "/" + joined
	}
	return joined
}
