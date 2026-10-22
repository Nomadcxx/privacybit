// Command privacybit-installer installs the privacybit user watcher or timer.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/Nomadcxx/privacybit/internal/installer"
)

func main() {
	mode := flag.String("mode", "", "install mode: watcher or timer")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "privacybit-installer: unexpected positional arguments")
		os.Exit(2)
	}
	if *mode != "watcher" && *mode != "timer" {
		fmt.Fprintln(os.Stderr, "privacybit-installer: --mode must be watcher or timer")
		os.Exit(2)
	}
	configHome, err := installer.ConfigHome()
	if err != nil {
		fail(err)
	}
	binary, err := installer.FindPrivacybit()
	if err != nil {
		fail(err)
	}
	if err := installer.Install(context.Background(), configHome, binary, *mode, nil); err != nil {
		fail(err)
	}
	fmt.Printf("Installed privacybit %s mode in the user systemd manager\n", *mode)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "privacybit-installer: "+err.Error())
	os.Exit(1)
}
