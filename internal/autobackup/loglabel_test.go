package autobackup

import (
	"strings"
	"testing"
)

func TestLogPrefixPadsShortPathsToFixedWidth(t *testing.T) {
	got := LogPrefix("/src")
	if runeLen(got) != maxLogPrefixLen {
		t.Fatalf("prefix length got %d want %d: %q", runeLen(got), maxLogPrefixLen, got)
	}
	if strings.HasPrefix(got, "[ ") {
		t.Fatalf("path should start immediately after bracket: %q", got)
	}
	if !strings.HasPrefix(got, "[/src ") {
		t.Fatalf("short path was not left aligned: %q", got)
	}
	if !strings.HasSuffix(got, " ]:") {
		t.Fatalf("short path was not right padded inside brackets: %q", got)
	}
}

func TestLogPrefixShortensLongWindowsPath(t *testing.T) {
	path := `C:\home\user\projects\example-repo\examples\SampleApplication`
	got := LogPrefix(path)
	if runeLen(got) != maxLogPrefixLen {
		t.Fatalf("prefix length got %d want %d: %q", runeLen(got), maxLogPrefixLen, got)
	}
	if strings.HasPrefix(got, "[ ") {
		t.Fatalf("path should start immediately after bracket: %q", got)
	}
	if !strings.Contains(got, `C:\home\user\`) || !strings.Contains(got, `...`) {
		t.Fatalf("prefix does not keep expected start: %q", got)
	}
	if !strings.HasSuffix(got, `examples\SampleApplication]:`) {
		t.Fatalf("prefix does not keep expected tail: %q", got)
	}
}

func TestShortPathLabelTrimsVeryLongTailToMax(t *testing.T) {
	got := ShortPathLabel(`/very/long/path/with/a/reallyreallyreallylongfolder/file.ext`, 20)
	if runeLen(got) != 20 {
		t.Fatalf("label length got %d want 20: %q", runeLen(got), got)
	}
	if !strings.HasPrefix(got, "/very/long/path/") || !strings.Contains(got, "...") {
		t.Fatalf("label does not keep start and ellipsis: %q", got)
	}
}
