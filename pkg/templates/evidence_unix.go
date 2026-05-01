//go:build !windows

package templates

import (
	"fmt"
	"syscall"
)

// sameDevice returns true iff `a` and `b` resolve to the same filesystem
// device. Used to enforce EV-N-7 (cross-device --out rejection): the
// sibling-tmp parent and the --out target must be on the same device for
// the final atomic rename to succeed. darwin and linux both populate
// syscall.Stat_t.Dev for this check.
func sameDevice(a, b string) (bool, error) {
	var sa, sb syscall.Stat_t
	if err := syscall.Stat(a, &sa); err != nil {
		return false, fmt.Errorf("templates: stat %q: %w", a, err)
	}
	if err := syscall.Stat(b, &sb); err != nil {
		return false, fmt.Errorf("templates: stat %q: %w", b, err)
	}
	return sa.Dev == sb.Dev, nil
}
