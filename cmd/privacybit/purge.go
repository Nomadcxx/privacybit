package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Nomadcxx/privacybit/internal/engine"
	"github.com/Nomadcxx/privacybit/internal/helper"
	"github.com/Nomadcxx/privacybit/internal/runner"
	"github.com/spf13/cobra"
)

func newPurgeCommand(userFragments *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "purge <entry-id>",
		Short: "Review and remove a catalogue entry using the root helper",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entryID := args[0]
			set, err := loadCatalogue(userFragments)
			if err != nil {
				return commandError{code: 2, err: err}
			}
			if set.Entry(entryID) == nil {
				return commandError{code: 1, err: fmt.Errorf("unknown catalogue entry %q", entryID)}
			}
			tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
			if err != nil {
				return commandError{code: 1, err: fmt.Errorf("purge requires a controlling terminal: %w", err)}
			}
			defer tty.Close()

			snapshot, err := engine.Run(cmd.Context(), set, engine.Options{Network: false})
			if err != nil {
				return commandError{code: 2, err: fmt.Errorf("audit before purge: %w", err)}
			}
			auditAt := snapshot.GeneratedAt.Format(time.RFC3339Nano)
			session, err := runner.Start(cmd.Context(), "", "", cmd.ErrOrStderr())
			if err != nil {
				return commandError{code: 2, err: err}
			}
			defer session.Close()

			planned, err := session.Request(helper.Request{
				Op: "plan-purge", Entry: entryID, AuditGeneratedAt: auditAt,
			})
			if err != nil {
				return commandError{code: 2, err: err}
			}
			if planned.Plan == nil {
				return commandError{code: 3, err: fmt.Errorf("helper returned no purge plan")}
			}
			if err := renderPurgePlan(cmd.OutOrStdout(), *planned.Plan); err != nil {
				return commandError{code: 3, err: err}
			}
			confirmed, err := confirmPurge(tty, entryID)
			if err != nil {
				return commandError{code: 1, err: err}
			}
			if !confirmed {
				fmt.Fprintln(cmd.OutOrStdout(), "Cancelled; no packages were removed.")
				return nil
			}
			if _, err := session.Request(helper.Request{
				Op: "purge-entry", Entry: entryID, Token: planned.Plan.Token,
				AuditGeneratedAt: auditAt,
			}); err != nil {
				return commandError{code: 2, err: err}
			}
			fmt.Fprintln(cmd.OutOrStdout(), "The reviewed package set was removed.")
			return nil
		},
	}
}

func purgeConfirmationMatches(entry, input string) bool {
	return entry != "" && input == "PURGE "+entry
}

func confirmPurge(tty io.ReadWriter, entry string) (bool, error) {
	if _, err := fmt.Fprintf(tty, "Type PURGE %s to remove exactly this reviewed package set: ", entry); err != nil {
		return false, err
	}
	line, err := bufio.NewReader(tty).ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	return purgeConfirmationMatches(entry, line), nil
}

func renderPurgePlan(w io.Writer, plan helper.Plan) error {
	if _, err := fmt.Fprintf(w, "Removal plan for %s\n", plan.Entry); err != nil {
		return err
	}
	if plan.AuditGeneratedAt != "" {
		if _, err := fmt.Fprintf(w, "Audit context: %s\n", plan.AuditGeneratedAt); err != nil {
			return err
		}
	}
	for _, item := range plan.Removals {
		if _, err := fmt.Fprintf(w, "  %s — %.1f MiB\n", item.Package, float64(item.InstalledBytes)/(1<<20)); err != nil {
			return err
		}
		if len(item.RequiredBy) > 0 {
			if _, err := fmt.Fprintf(w, "    Required by: %s\n", strings.Join(item.RequiredBy, ", ")); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintf(w, "Total reclaim: %.1f MiB\n", float64(plan.ReclaimBytes)/(1<<20)); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "This plan expires at %s. The package state will be checked again before removal.\n",
		plan.ExpiresAt.Local().Format(time.RFC3339))
	return err
}
