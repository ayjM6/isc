package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ayjM6/isc/internal/buildinfo"
)

func newVersionCmd(info buildinfo.Info) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the isc version, the ImageSetConfiguration version, the oc-mirror commit and the Go version",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprint(cmd.OutOrStdout(), info.String())
			return err
		},
	}
}
