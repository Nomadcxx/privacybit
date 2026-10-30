package main

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

var (
	buildVersion = "devel"
	buildCommit  string
)

func newVersionCommand(userFragments *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the program version and catalogue revision",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			set, err := loadCatalogue(userFragments)
			if err != nil {
				return err
			}
			version, commit := cliBuildIdentity()
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Version: %s\nCommit: %s\nCatalogue revision: %s\n",
				version, commit, set.Revision)
			return err
		},
	}
}

func cliBuildIdentity() (version, commit string) {
	version, commit = buildVersion, buildCommit
	if info, ok := debug.ReadBuildInfo(); ok {
		if commit == "" {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					commit = setting.Value
					break
				}
			}
		}
	}
	if version == "" {
		version = "devel"
	}
	if commit == "" {
		commit = "unknown"
	}
	return version, commit
}
