package main

import (
	"fmt"
	"os"

	"github.com/Nomadcxx/privacybit/internal/catalog"
)

// useColour is resolved once at startup. It gates every escape sequence the
// CLI emits, not just the grade colour: a partially-coloured output is harder
// to read than an uncoloured one, because the reader has to work out which
// parts carry meaning.
//
// NO_COLOR is honoured because a user who sets it is telling us the terminal
// cannot be relied on, and because the D2 contract promises the glyph and word
// are what survive on a pipe.
var useColour = isTerminal(os.Stdout)

func isTerminal(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// userFragmentPath is a thin wrapper so the commands do not each have to decide
// what to do when the config directory cannot be resolved. A missing or
// unreadable fragment directory is not an error: the shipped catalogue still
// loads and the audit still runs.
func userFragmentPath() (string, error) {
	dir, err := catalog.UserFragmentDir()
	if err != nil {
		return "", fmt.Errorf("locate user catalogue directory: %w", err)
	}
	return dir, nil
}

func cmdStdout() *os.File { return os.Stdout }
