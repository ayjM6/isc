package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ayjM6/isc/internal/buildinfo"
)

func run(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = execute("1.2.3", &out, &errOut, args)
	return code, out.String(), errOut.String()
}

func TestVersion(t *testing.T) {
	want := buildinfo.New("1.2.3").String()
	for _, args := range [][]string{{"--version"}, {"version"}} {
		code, stdout, stderr := run(args...)
		assert.Equal(t, exitOK, code, args)
		assert.Equal(t, want, stdout, args)
		assert.Empty(t, stderr, args)
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"no-such-command"},
		{"--no-such-flag"},
		{"version", "extra-argument"},
	} {
		code, stdout, stderr := run(args...)
		assert.Equal(t, exitUsage, code, args)
		assert.Empty(t, stdout, args)
		assert.Contains(t, stderr, "error: ", args)
		assert.Contains(t, stderr, "Run 'isc --help' for usage.", args)
	}
}

func TestHelp(t *testing.T) {
	for _, args := range [][]string{{}, {"--help"}} {
		code, stdout, _ := run(args...)
		assert.Equal(t, exitOK, code, args)
		assert.Contains(t, stdout, "Usage:", args)
		assert.Contains(t, stdout, "--version", args)
	}
}
