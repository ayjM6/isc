package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/ayjM6/isc/internal/buildinfo"
)

// Exit codes shared by every command.
const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
)

// globalOptions are the flags every command accepts.
type globalOptions struct {
	verbose bool
}

// usageError marks a mistake in how isc was invoked, so it exits with exitUsage.
type usageError struct{ err error }

func (e usageError) Error() string { return e.err.Error() }
func (e usageError) Unwrap() error { return e.err }

// usageArgs wraps a cobra argument validator so its errors count as usage errors.
func usageArgs(validate cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := validate(cmd, args); err != nil {
			return usageError{err}
		}
		return nil
	}
}

// execute runs isc with args and returns the process exit code.
func execute(version string, stdout, stderr io.Writer, args []string) int {
	root := newRootCmd(buildinfo.New(version))
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)

	err := root.Execute()
	if err == nil {
		return exitOK
	}
	fmt.Fprintf(stderr, "error: %v\n", err)
	if errors.As(err, new(usageError)) {
		fmt.Fprintf(stderr, "Run '%s --help' for usage.\n", root.CommandPath())
		return exitUsage
	}
	return exitFailure
}

func newRootCmd(info buildinfo.Info) *cobra.Command {
	opts := &globalOptions{}
	root := &cobra.Command{
		Use:   "isc",
		Short: "Build, check and edit oc-mirror v2 ImageSetConfigurations",
		// cobra only validates Args on a runnable command, so the root prints help
		// itself; that lets usageArgs report unknown commands as usage errors.
		Args:          usageArgs(cobra.NoArgs),
		RunE:          func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
		Version:       info.Version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetVersionTemplate(info.String())
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err} })
	root.PersistentFlags().BoolVarP(&opts.verbose, "verbose", "v", false, "print decisions and progress on stderr")

	root.AddCommand(newVersionCmd(info))
	return root
}
