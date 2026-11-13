package cli

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Version is the module version of the binary: a release or pseudo-version
// when installed with go install or go run pkg@version, "(devel)" from a
// checkout.
func Version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

// NewVersionCmd is the version command of binary.
func NewVersionCmd(binary string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the module version",
		Args:  Args(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), Version())
			return err
		},
	}
}

// AddVersion gives root the version command and the --version flag, which
// both print [Version] alone.
func AddVersion(root *cobra.Command) {
	root.Version = Version()
	root.SetVersionTemplate("{{.Version}}\n")
	root.AddCommand(NewVersionCmd(root.Name()))
}
