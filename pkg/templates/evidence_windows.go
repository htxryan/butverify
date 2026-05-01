//go:build windows

package templates

// sameDevice on Windows is a permissive stub. Windows lacks the unix
// `Stat_t.Dev` field, and `os.Rename` across volumes works differently
// than on POSIX (it can succeed via copy+delete in some cases). Returning
// `true` skips the EV-N-7 cross-device flag-parse rejection on Windows;
// the rename itself will surface a clear error if it actually fails.
//
// Agents using `bv evidence` are typically on Linux/macOS (the Claude
// Code, Cursor, Codex audience), so this Windows stub keeps the binary
// portable without re-implementing the device-id check via syscalls
// like `GetVolumeInformationByHandle` for the v1 release.
func sameDevice(a, b string) (bool, error) {
	return true, nil
}
