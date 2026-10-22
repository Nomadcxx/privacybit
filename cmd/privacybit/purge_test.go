package main

import "testing"

func TestRootCommandExposesAttendedPurge(t *testing.T) {
	for _, command := range newRootCommand().Commands() {
		if command.Name() == "purge" {
			return
		}
	}
	t.Fatal("root command has no purge command")
}

func TestPurgeConfirmationRequiresExactEntryID(t *testing.T) {
	for _, test := range []struct {
		input string
		want  bool
	}{
		{input: "PURGE chrome", want: true},
		{input: "PURGE Chrome", want: false},
		{input: "yes", want: false},
		{input: "PURGE chrome ", want: false},
	} {
		if got := purgeConfirmationMatches("chrome", test.input); got != test.want {
			t.Errorf("confirmation %q = %t, want %t", test.input, got, test.want)
		}
	}
}
