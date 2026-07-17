//go:build !windows

package autobackup

import (
	"context"
	"io"
)

func PrepareIdentityFile(_ context.Context, _ string, _ io.Writer) error {
	return nil
}
