// Command privacybit-helper serves a closed, short-lived privileged session.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/Nomadcxx/privacybit/internal/catalog"
	"github.com/Nomadcxx/privacybit/internal/helper"
)

func main() {
	if err := run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "privacybit-helper:", err)
		os.Exit(2)
	}
}

func run(input io.Reader, output io.Writer) error {
	set, err := catalog.Load(catalog.LoadOptions{UserFragments: false})
	if err != nil {
		return fmt.Errorf("load embedded catalogue: %w", err)
	}
	var log *helper.AuditLog
	logRecord := func(record helper.AuditRecord) error {
		if log == nil {
			uidText := os.Getenv("SUDO_UID")
			uid, err := strconv.Atoi(uidText)
			if err != nil || uid < 0 {
				return errors.New("SUDO_UID is missing or invalid; refusing an unlogged mutation")
			}
			log, err = helper.OpenAuditLog(uid)
			if err != nil {
				return err
			}
		}
		return log.Record(record)
	}
	defer func() {
		if log != nil {
			_ = log.Close()
		}
	}()
	return helper.Serve(context.Background(), input, output, helper.Options{
		Catalog: set,
		Log:     logRecord,
	})
}
