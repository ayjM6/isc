package buildinfo

import (
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ayjM6/isc/internal/ocmirror"
)

func TestNew(t *testing.T) {
	info := New("1.2.3")
	assert.Equal(t, Info{
		Version:  "1.2.3",
		ISC:      "mirror.openshift.io/v2alpha1",
		OcMirror: ocmirror.Commit,
		Go:       runtime.Version(),
	}, info)
	assert.Equal(t, "isc 1.2.3\n"+
		"ImageSetConfiguration mirror.openshift.io/v2alpha1\n"+
		"oc-mirror "+ocmirror.Commit+" (copied code)\n"+
		runtime.Version()+"\n", info.String())
}

func TestResolveVersion(t *testing.T) {
	buildInfo := func(version string) func() (*debug.BuildInfo, bool) {
		return func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{Main: debug.Module{Version: version}}, true
		}
	}
	noBuildInfo := func() (*debug.BuildInfo, bool) { return nil, false }

	assert.Equal(t, "1.2.3", resolveVersion("1.2.3", buildInfo("v9.9.9")), "an ldflags version wins")
	assert.Equal(t, "v0.3.0", resolveVersion("dev", buildInfo("v0.3.0")), "go install records the module version")
	assert.Equal(t, "dev", resolveVersion("dev", buildInfo("(devel)")), "a local build has no module version")
	assert.Equal(t, "dev", resolveVersion("dev", noBuildInfo))
}
