//go:build !windows

package daemon

import "io/fs"

func userOnlyFile(mode fs.FileMode) bool {
	return mode.Perm()&0o077 == 0
}
