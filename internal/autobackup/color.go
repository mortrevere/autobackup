package autobackup

import (
	"os"
	"runtime"
	"strings"
)

type Color string

const (
	ColorRed    Color = "\033[31m"
	ColorGreen  Color = "\033[32m"
	ColorYellow Color = "\033[33m"
	ColorBlue   Color = "\033[34m"
	ColorWhite  Color = "\033[37m"
	colorReset  Color = "\033[0m"
)

func Colorize(color Color, text string) string {
	if !ColorEnabled() {
		return text
	}
	return string(color) + text + string(colorReset)
}

func ColorEnabled() bool {
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled {
		return false
	}
	if _, forced := os.LookupEnv("FORCE_COLOR"); forced {
		return true
	}
	if runtime.GOOS == "windows" {
		term := strings.ToLower(os.Getenv("TERM"))
		return os.Getenv("WT_SESSION") != "" ||
			os.Getenv("ANSICON") != "" ||
			strings.EqualFold(os.Getenv("ConEmuANSI"), "ON") ||
			strings.Contains(term, "xterm") ||
			strings.Contains(term, "cygwin") ||
			strings.Contains(term, "msys") ||
			strings.Contains(term, "ansi")
	}
	return os.Getenv("TERM") != "dumb"
}
