package markdown

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stacklok/modelith/internal/model"
)

// firstDiff reports the first line where want and got differ, so a golden
// failure points at the change instead of just saying "they differ."
func firstDiff(want, got string) string {
	wl := strings.Split(want, "\n")
	gl := strings.Split(got, "\n")
	n := len(wl)
	if len(gl) > n {
		n = len(gl)
	}
	for i := 0; i < n; i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			return fmt.Sprintf("first difference at line %d:\n  want: %q\n  got:  %q", i+1, w, g)
		}
	}
	return "(no line-level difference found)"
}

// TestRenderInvariantsSection checks the model-level invariants render as a
// top-level "## Invariants" section, and that the section is omitted entirely
// when there are none (per-entity invariants render with their entity instead).
func TestRenderInvariantsSection(t *testing.T) {
	with := &model.Model{
		Entities: map[string]model.Entity{
			"Project": {Definition: "A container."},
		},
		Invariants: []model.Invariant{
			{ID: "cross-entity-rule", Statement: "Spans the `Project` and the `Policy`."},
		},
	}
	got := Render(with)
	if !strings.Contains(got, "## Invariants\n") {
		t.Fatalf("expected a top-level Invariants section, got:\n%s", got)
	}
	if !strings.Contains(got, "- **cross-entity-rule** — Spans the `Project` and the `Policy`.") {
		t.Fatalf("expected the invariant bullet, got:\n%s", got)
	}

	without := &model.Model{
		Entities: map[string]model.Entity{
			"Project": {Definition: "A container."},
		},
	}
	if strings.Contains(Render(without), "## Invariants") {
		t.Fatalf("did not expect an Invariants section when there are none:\n%s", Render(without))
	}
}

// TestRenderEntity_DerivedMarker checks that a derived entity shows a clear
// "Derived" marker with its derivation when present, and a generic marker
// when derivation is omitted (mirroring the derived-attribute rendering).
func TestRenderEntity_DerivedMarker(t *testing.T) {
	withDerivation := &model.Model{
		Entities: map[string]model.Entity{
			"Leaderboard": {
				Definition: "A ranked view over current scores.",
				Derived:    true,
				Derivation: "Computed on demand from `Score` records.",
			},
		},
	}
	got := Render(withDerivation)
	if !strings.Contains(got, "**Derived:** Computed on demand from `Score` records.\n\n") {
		t.Fatalf("expected a derivation-specific marker, got:\n%s", got)
	}

	withoutDerivation := &model.Model{
		Entities: map[string]model.Entity{
			"Leaderboard": {
				Definition: "A ranked view over current scores.",
				Derived:    true,
			},
		},
	}
	got = Render(withoutDerivation)
	if !strings.Contains(got, "**Derived.** Computed on demand from other state; never persisted.\n\n") {
		t.Fatalf("expected a generic derived marker when derivation is omitted, got:\n%s", got)
	}

	notDerived := &model.Model{
		Entities: map[string]model.Entity{
			"Leaderboard": {Definition: "A ranked view over current scores."},
		},
	}
	if strings.Contains(Render(notDerived), "**Derived") {
		t.Fatalf("did not expect a derived marker on a non-derived entity:\n%s", Render(notDerived))
	}
}

// TestGoldenExample renders the committed example and compares it to the
// checked-in Markdown. This is the same invariant `modelith render --check` enforces
// in CI: if you change the renderer or the example YAML, regenerate the .md.
func TestGoldenExample(t *testing.T) {
	const (
		src    = "../../../examples/example.modelith.yaml"
		golden = "../../../examples/example.modelith.md"
	)
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	m, err := model.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	got := Render(m)

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("rendered output does not match %s.\nRegenerate with: go run ./cmd/modelith render %s\n%s",
			golden, src, firstDiff(string(want), got))
	}
}

// TestRenderEntity_SubtypeHierarchy checks that a child names its supertype and
// a parent lists its subtypes.
func TestRenderEntity_SubtypeHierarchy(t *testing.T) {
	m := &model.Model{Entities: map[string]model.Entity{
		"PaymentMethod": {Definition: "A way to pay."},
		"Card":          {Definition: "A card.", SubtypeOf: "PaymentMethod"},
		"BankTransfer":  {Definition: "A transfer.", SubtypeOf: "PaymentMethod"},
	}}
	got := Render(m)
	if !strings.Contains(got, "**Subtype of** `PaymentMethod`") {
		t.Errorf("expected child to name its supertype:\n%s", got)
	}
	// names render alphabetically, so BankTransfer precedes Card.
	if !strings.Contains(got, "**Subtypes** — `BankTransfer`, `Card`") {
		t.Errorf("expected parent to list its subtypes:\n%s", got)
	}
}
