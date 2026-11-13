package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/spf13/cobra"
)

// Args wraps a Cobra argument validator so its error is a [UsageError].
func Args(v cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := v(cmd, args); err != nil {
			return &UsageError{Msg: err.Error()}
		}
		return nil
	}
}

// Group makes cmd a command that only has subcommands: with no subcommand it
// prints its help to stderr and is a usage error, with an unknown one it is a
// usage error naming it.
func Group(cmd *cobra.Command) *cobra.Command {
	cmd.Args = Args(cobra.NoArgs)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		fmt.Fprint(cmd.ErrOrStderr(), cmd.UsageString())
		return Usagef("%s needs a command", cmd.CommandPath())
	}
	return cmd
}

// Execute runs root and returns the exit code: 0 on success, 2 for a
// [UsageError], a flag that does not parse, a wrong argument count or a
// missing required flag, 1 for any other error. An error is printed to
// stderr as "<command path>: <error>", without the usage text. Output of
// --help goes where the caller pointed root.SetOut, stdout by default.
func Execute(root *cobra.Command, stderr io.Writer) int {
	root.SilenceUsage = true
	root.SilenceErrors = true
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &UsageError{Msg: err.Error()}
	})
	root.SetHelpCommand(helpCmd())
	// A running command (serve) stops on Ctrl-C through cmd.Context().
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	cmd, err := root.ExecuteContextC(ctx)
	if err == nil {
		return ExitOK
	}
	if cmd == nil {
		cmd = root
	}
	fmt.Fprintf(stderr, "%s: %s\n", cmd.CommandPath(), err)
	if isUsage(err) {
		return ExitUsage
	}
	return ExitFailure
}

// isUsage reports whether err is a usage error. Cobra builds the errors of
// required and mutually exclusive flags itself, as plain errors, so those are
// recognised by their text.
func isUsage(err error) bool {
	var u *UsageError
	if errors.As(err, &u) {
		return true
	}
	msg := err.Error()
	return strings.HasPrefix(msg, "required flag(s)") ||
		strings.HasPrefix(msg, "if any flags in the group") ||
		strings.HasPrefix(msg, "at least one of the flags in the group")
}

// helpCmd is the help command: Cobra's own exits 0 for an unknown topic, this
// one makes it a usage error.
func helpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "help [command]",
		Short: "Help about a command",
		RunE: func(cmd *cobra.Command, args []string) error {
			target, rest, err := cmd.Root().Find(args)
			if err != nil || target == nil || len(rest) > 0 {
				return Usagef("unknown command %q", strings.Join(args, " "))
			}
			return target.Help()
		},
	}
}
