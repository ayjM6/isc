// Package buildinfo describes the isc binary: its version, the ImageSetConfiguration
// API version it reads and writes, the oc-mirror commit its copied code comes from,
// and the Go version it was built with.
package buildinfo

import (
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/ayjM6/isc/internal/ocmirror"
	"github.com/ayjM6/isc/internal/ocmirror/api/v2alpha1"
)

// Info is what `isc --version` reports.
type Info struct {
	Version  string // isc's version
	ISC      string // ImageSetConfiguration apiVersion isc reads and writes
	OcMirror string // oc-mirror commit of the code in internal/ocmirror
	Go       string // Go version the binary was built with
}

// New describes this binary. version is the value set at build time with
// -ldflags; "dev" falls back to the module version recorded by `go install`.
func New(version string) Info {
	return Info{
		Version:  resolveVersion(version, debug.ReadBuildInfo),
		ISC:      v2alpha1.GroupVersion.String(),
		OcMirror: ocmirror.Commit,
		Go:       runtime.Version(),
	}
}

func resolveVersion(version string, readBuildInfo func() (*debug.BuildInfo, bool)) string {
	if version != "dev" {
		return version
	}
	if info, ok := readBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

// String is the text `isc --version` prints.
func (i Info) String() string {
	return fmt.Sprintf("isc %s\nImageSetConfiguration %s\noc-mirror %s (copied code)\n%s\n",
		i.Version, i.ISC, i.OcMirror, i.Go)
}
