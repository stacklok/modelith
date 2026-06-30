package model

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stacklok/modelith/internal/schema"
)

// TestSchemaStructSync guards against drift between the canonical JSON Schema
// and the Go structs: every property declared in a schema object must have a
// matching struct field (by json tag), and vice versa. This catches the common
// failure — a field added to one but not the other.
//
// It does NOT check types (e.g. a schema enum vs a Go string); the schema
// remains the source of truth for value-level constraints. If this test fails,
// reconcile internal/schema/v1/modelith.schema.json and internal/model/model.go.
func TestSchemaStructSync(t *testing.T) {
	var root map[string]any
	if err := json.Unmarshal(schema.JSON(), &root); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		path []string // navigation into the schema to the object with "properties"
		typ  reflect.Type
	}{
		{"Model", nil, reflect.TypeOf(Model{})},
		{"Entity", []string{"$defs", "entity"}, reflect.TypeOf(Entity{})},
		{"Relationship", []string{"$defs", "relationship"}, reflect.TypeOf(Relationship{})},
		{"Attribute", []string{"$defs", "attribute"}, reflect.TypeOf(Attribute{})},
		{"Scenario", []string{"$defs", "scenario"}, reflect.TypeOf(Scenario{})},
		{"Enum", []string{"$defs", "enum"}, reflect.TypeOf(Enum{})},
		{"EnumValue", []string{"$defs", "enumValue"}, reflect.TypeOf(EnumValue{})},
		{"Action", []string{"$defs", "actionObject"}, reflect.TypeOf(Action{})},
		{"Invariant", []string{"$defs", "invariant"}, reflect.TypeOf(Invariant{})},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			schemaProps := propertyNames(t, root, c.path)
			structFields := jsonFieldNames(c.typ)
			if !reflect.DeepEqual(schemaProps, structFields) {
				t.Errorf("%s out of sync:\n  schema properties: %v\n  struct json fields: %v",
					c.name, sortedKeys(schemaProps), sortedKeys(structFields))
			}
		})
	}
}

func propertyNames(t *testing.T, root map[string]any, path []string) map[string]bool {
	t.Helper()
	cur := root
	for _, p := range path {
		next, ok := cur[p].(map[string]any)
		if !ok {
			t.Fatalf("schema path %v: missing %q", path, p)
		}
		cur = next
	}
	props, ok := cur["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema path %v: no \"properties\"", path)
	}
	out := map[string]bool{}
	for k := range props {
		out[k] = true
	}
	return out
}

func jsonFieldNames(rt reflect.Type) map[string]bool {
	out := map[string]bool{}
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "" || name == "-" {
			continue
		}
		out[name] = true
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
