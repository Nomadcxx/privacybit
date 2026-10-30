package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Nomadcxx/privacybit/internal/catalog"
)

func TestVersionCommandPrintsVersionCommitAndCatalogueRevision(t *testing.T) {
	cmd := newRootCommand()
	cmd.SetArgs([]string{"version", "--user-catalog=false"})
	var output, stderr bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute version: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	for _, label := range []string{"Version: ", "Commit: ", "Catalogue revision: "} {
		found := false
		for _, line := range lines {
			if strings.HasPrefix(line, label) && strings.TrimSpace(strings.TrimPrefix(line, label)) != "" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("version output missing a value for %q: %q", label, output.String())
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("version wrote to stderr: %s", stderr.String())
	}
	if !strings.Contains(output.String(), "Version: devel\n") {
		t.Errorf("source build version = %q, want devel", output.String())
	}
	set, err := catalog.Load(catalog.LoadOptions{UserFragments: false})
	if err != nil {
		t.Fatalf("load shipped catalogue: %v", err)
	}
	if !strings.Contains(output.String(), "Catalogue revision: "+set.Revision) {
		t.Errorf("catalogue revision = %q, want shipped revision %q", output.String(), set.Revision)
	}
}
