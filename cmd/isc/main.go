// Command isc builds, checks and edits oc-mirror v2 ImageSetConfigurations.
//
// This package only parses the command line (cobra commands, flags, exit
// codes); the work itself happens in the internal packages.
package main

import (
	"os"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(execute(version, os.Stdout, os.Stderr, os.Args[1:]))
}
