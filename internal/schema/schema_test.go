package schema

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestURLConsistency guards the three places the canonical schema URL appears
// from drifting apart: the embedded schema's "$id", the Go URL constant
// (URLFor), and the "# yaml-language-server: $schema=" header in the worked
// example. SCHEMA-1: a mismatch here means committed models point at a schema
// that no longer matches the binary's notion of identity.
func TestURLConsistency(t *testing.T) {
	// Every embedded schema's $id must equal the URL we compute for its version,
	// and its version const must match the registry key.
	for _, v := range SupportedVersions() {
		var doc map[string]any
		if err := json.Unmarshal(JSONFor(v), &doc); err != nil {
			t.Fatalf("%s: unmarshal embedded schema: %v", v, err)
		}
		if id, _ := doc["$id"].(string); id != URLFor(v) {
			t.Errorf("%s: $id %q != URLFor(%q) %q", v, id, v, URLFor(v))
		}
		if props, ok := doc["properties"].(map[string]any); ok {
			if ver, ok := props["version"].(map[string]any); ok {
				if c, _ := ver["const"].(string); c != "" && c != v {
					t.Errorf("%s: version.const %q != version key %q", v, c, v)
				}
			}
		}
	}

	// URL must be the current version's URL.
	if URL != URLFor(Current) {
		t.Errorf("URL %q != URLFor(Current) %q", URL, URLFor(Current))
	}

	// The worked example's header must point at the current canonical URL.
	const example = "../../examples/example.modelith.yaml"
	data, err := os.ReadFile(example)
	if err != nil {
		t.Fatalf("reading %s: %v", example, err)
	}
	header := firstSchemaHeader(string(data))
	if header == "" {
		t.Fatalf("%s: no '# yaml-language-server: $schema=' header found", example)
	}
	if header != URL {
		t.Errorf("%s header %q != canonical URL %q", example, header, URL)
	}
}

// TestCompileCurrent compiles the current embedded schema and validates a
// minimal document against it, exercising Compile()/CompileVersion(Current).
func TestCompileCurrent(t *testing.T) {
	sch, err := Compile()
	if err != nil {
		t.Fatalf("Compile() errored: %v", err)
	}
	doc := map[string]any{
		"kind":    "DomainModel",
		"version": Current,
		"entities": map[string]any{
			"Thing": map[string]any{"definition": "A thing that exists."},
		},
	}
	if err := sch.Validate(doc); err != nil {
		t.Fatalf("minimal valid document failed validation: %v", err)
	}
}

// TestCompileVersionUnknown checks the error path for a version this build does
// not embed.
func TestCompileVersionUnknown(t *testing.T) {
	if _, err := CompileVersion("v999"); err == nil {
		t.Fatal("expected an error compiling an unsupported version, got nil")
	}
	if JSONFor("v999") != nil {
		t.Fatal("JSONFor on an unknown version should return nil")
	}
	if Supported("v999") {
		t.Fatal("Supported should be false for an unknown version")
	}
}

// firstSchemaHeader returns the URL from the first
// "# yaml-language-server: $schema=" line, or "" if none.
func firstSchemaHeader(content string) string {
	const marker = "# yaml-language-server: $schema="
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, marker) {
			return strings.TrimSpace(strings.TrimPrefix(line, marker))
		}
	}
	return ""
}
