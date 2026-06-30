// Package schema holds the canonical JSON Schemas for a Stacklok domain model,
// one per format version, and helpers to compile them. The schema files
// themselves (vN/modelith.schema.json) are the source of truth; each is also
// published by URL (see URLFor) so editors can use it via a
// "# yaml-language-server: $schema=" header.
//
// This package is internal on purpose: modelith's contract is the CLI and the
// published JSON Schema, not a Go API. The schema living under internal/ has no
// effect on the file being reachable by URL — internal/ is a Go-compiler
// visibility rule only — it is published to modelith.sh by the release pipeline.
package schema

import (
	"bytes"
	_ "embed"
	"fmt"
	"sort"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed v1/modelith.schema.json
var rawV1 []byte

// Current is the format version this build of modelith writes and renders. It
// is the version whose schema is used for the "# yaml-language-server" header
// modelith emits and the default for commands that print the schema.
const Current = "v1"

// registry maps each supported format version to its embedded schema bytes.
// Evolution is additive: to add a version, drop its schema under a new vN/
// directory, embed it, and add an entry here. Existing version entries are
// never touched, so models pinned to an older version keep validating exactly
// as they did before — adding a version is not a breaking change.
var registry = map[string][]byte{
	"v1": rawV1,
}

// urlBase is the published location of the canonical schemas. The version is the
// final path segment (".../v1.json"). The schema's $id and the YAML header use
// these URLs as identity; they need not resolve for the CLI to validate, since
// Compile registers the embedded bytes against the URL locally.
const urlBase = "https://modelith.sh/schema/domain-model/"

// URLFor returns the canonical published URL ($id) for a format version.
func URLFor(version string) string {
	return urlBase + version + ".json"
}

// URL is the canonical published URL for the current schema version. It is the
// value modelith writes into a "# yaml-language-server: $schema=" header.
var URL = URLFor(Current)

// Supported reports whether version is a format version this build understands.
func Supported(version string) bool {
	_, ok := registry[version]
	return ok
}

// SupportedVersions returns the known versions in a stable order, for error
// messages and help text.
func SupportedVersions() []string {
	vs := make([]string, 0, len(registry))
	for v := range registry {
		vs = append(vs, v)
	}
	sort.Strings(vs)
	return vs
}

// JSON returns the raw bytes of the current canonical JSON Schema.
func JSON() []byte { return JSONFor(Current) }

// JSONFor returns the raw bytes of the schema for a given version, or nil if the
// version is unknown.
func JSONFor(version string) []byte {
	raw, ok := registry[version]
	if !ok {
		return nil
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return out
}

// Compile parses and compiles the current embedded schema, returning a validator.
func Compile() (*jsonschema.Schema, error) { return CompileVersion(Current) }

// CompileVersion parses and compiles the embedded schema for a given format
// version. It returns an error if the version is not supported by this build.
func CompileVersion(version string) (*jsonschema.Schema, error) {
	raw, ok := registry[version]
	if !ok {
		return nil, fmt.Errorf("unsupported schema version %q", version)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parsing embedded schema %s: %w", version, err)
	}
	url := URLFor(version)
	c := jsonschema.NewCompiler()
	if err := c.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("adding schema resource %s: %w", version, err)
	}
	sch, err := c.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("compiling schema %s: %w", version, err)
	}
	return sch, nil
}
