// Package isc loads ImageSetConfigurations with oc-mirror's own loader, so isc
// accepts exactly what oc-mirror accepts.
package isc

import (
	"errors"
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/ayjM6/isc/internal/ocmirror/api/v2alpha1"
	"github.com/ayjM6/isc/internal/ocmirror/config"
)

// Document is an ImageSetConfiguration loaded from a file: the typed view for
// reading, and the YAML node tree for edits that keep comments and key order.
type Document struct {
	Path   string
	Config v2alpha1.ImageSetConfiguration
	Node   *yaml.Node
}

// LoadError is a file that oc-mirror's loader rejects.
type LoadError struct {
	Path string
	Err  error
}

func (e *LoadError) Error() string { return fmt.Sprintf("%s: %v", e.Path, e.Err) }
func (e *LoadError) Unwrap() error { return e.Err }

// Problems splits oc-mirror's error into the separate problems it reports:
// Validate returns one aggregated error per failed check (validate.go).
func (e *LoadError) Problems() []error {
	var agg utilerrors.Aggregate
	if errors.As(e.Err, &agg) {
		return agg.Errors()
	}
	return []error{e.Err}
}

// Load reads path with oc-mirror's ReadConfig (strict YAML, kind and stanza
// checks, unknown fields rejected, and all of Validate), then parses the same
// file into a YAML node tree.
func Load(path string) (*Document, error) {
	loaded, err := config.ReadConfig(path, v2alpha1.ImageSetConfigurationKind)
	if err != nil {
		return nil, &LoadError{Path: path, Err: err}
	}
	// ReadConfig returns the configuration by value for this kind (load.go).
	cfg, ok := loaded.(v2alpha1.ImageSetConfiguration)
	if !ok {
		return nil, fmt.Errorf("%s: unexpected configuration type %T", path, loaded)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &Document{Path: path, Config: cfg, Node: &node}, nil
}
