//go:build !linux && !darwin

package vault

import "fmt"

// MoveToTrash is not implemented for this platform. Per Decision 10, refuse the
// operation instead of falling back to a permanent delete.
func MoveToTrash(absPath string) error {
	_ = absPath
	return fmt.Errorf("OS trash is unavailable on this platform; permanent delete is not performed")
}
