//go:build windows

package daemon

import "io/fs"

func userOnlyFile(_ fs.FileMode) bool {
	// Windows access control is enforced by the user's profile directory ACL.
	return true
}
