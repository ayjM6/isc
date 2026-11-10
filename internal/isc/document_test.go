package isc

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeISC(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "isc.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestLoadValid(t *testing.T) {
	path := writeISC(t, `# kept in the node tree
apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
mirror:
  operators:
    - catalog: registry.redhat.io/redhat/redhat-operator-index:v4.21
      packages:
        - name: compliance-operator
          minVersion: 1.6.0
`)
	doc, err := Load(path)
	require.NoError(t, err)

	assert.Equal(t, path, doc.Path)
	require.Len(t, doc.Config.Mirror.Operators, 1)
	op := doc.Config.Mirror.Operators[0]
	assert.Equal(t, "registry.redhat.io/redhat/redhat-operator-index:v4.21", op.Catalog)
	require.Len(t, op.Packages, 1)
	assert.Equal(t, "1.6.0", op.Packages[0].MinVersion)

	// yaml.v3 attaches a comment at the top of the file to the first key.
	require.NotNil(t, doc.Node)
	require.NotEmpty(t, doc.Node.Content)
	root := doc.Node.Content[0]
	require.NotEmpty(t, root.Content)
	assert.Contains(t, root.Content[0].HeadComment, "kept in the node tree")
}

func TestLoadRejectsMetadata(t *testing.T) {
	path := writeISC(t, `apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
metadata:
  name: cluster-a
mirror: {}
`)
	_, err := Load(path)
	require.Error(t, err)

	var loadErr *LoadError
	require.ErrorAs(t, err, &loadErr)
	assert.Equal(t, path, loadErr.Path)
	assert.Contains(t, err.Error(), path+": ")
	assert.Contains(t, err.Error(), `unknown field "metadata"`)
}

func TestLoadErrorProblems(t *testing.T) {
	// Two separate Validate checks fail: duplicate release channels, and a
	// blocked image that isn't a valid regular expression.
	path := writeISC(t, `apiVersion: mirror.openshift.io/v2alpha1
kind: ImageSetConfiguration
mirror:
  platform:
    channels:
      - name: stable-4.21
      - name: stable-4.21
  blockedImages:
    - name: "registry.example.com/(unclosed"
`)
	_, err := Load(path)
	require.Error(t, err)

	var loadErr *LoadError
	require.ErrorAs(t, err, &loadErr)
	problems := loadErr.Problems()
	require.Len(t, problems, 2)
	assert.Contains(t, problems[0].Error(), `release channel "stable-4.21": duplicate found in configuration`)
	assert.Contains(t, problems[1].Error(), "invalid regular expression")
}

func TestLoadErrorProblemsSingle(t *testing.T) {
	err := &LoadError{Path: "x.yaml", Err: errors.New("configuration missing `kind`")}
	assert.Equal(t, []error{err.Err}, err.Problems())
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	require.Error(t, err)
	assert.ErrorAs(t, err, new(*LoadError))
}
