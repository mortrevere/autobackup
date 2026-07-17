package autobackup

import "testing"

func TestConvertWindowsPathStyles(t *testing.T) {
	tests := []struct {
		name  string
		style WindowsPathStyle
		want  string
	}{
		{"native", PathNative, `C:\Users\Leo\Docs`},
		{"msys", PathMSYS, "/c/Users/Leo/Docs"},
		{"cygwin", PathCygwin, "/cygdrive/c/Users/Leo/Docs"},
		{"auto", PathAuto, "/c/Users/Leo/Docs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertWindowsPath(`C:\Users\Leo\Docs`, tt.style, "rsync.exe")
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestJoinRemote(t *testing.T) {
	got := JoinRemote("/tmp/disk/backup", "dest", ".")
	if got != "/tmp/disk/backup/dest" {
		t.Fatalf("got %q", got)
	}
}
