// Package cli is what the Cobra command trees of the weakpixel binaries share:
// the exit codes (0 ok, 1 failure, 2 usage), the error type that means a
// usage error, and the version command. Tool packages import only
// [UsageError] and [Usagef] from here; Cobra stays in the cmd packages.
package cli

import "fmt"

// Exit codes of every weakpixel binary.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// UsageError is an error that makes the command exit with [ExitUsage]: the
// command line was wrong, not the work it did.
type UsageError struct{ Msg string }

func (e *UsageError) Error() string { return e.Msg }

// Usagef returns a [UsageError] with a formatted message. A tool package
// returns it for a usage problem only the tool can detect.
func Usagef(format string, args ...any) error {
	return &UsageError{Msg: fmt.Sprintf(format, args...)}
}
