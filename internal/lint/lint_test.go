package lint

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// testModelPath stands in for the linted file's path. Only a model with
// imports cares what it is (they resolve relative to its directory), so the
// inline fixtures here pass it and a nil FileReader.
const testModelPath = "model.modelith.yaml"

func countBy(fs []Finding, sev Severity, cat Category) int {
	n := 0
	for _, f := range fs {
		if f.Severity == sev && f.Category == cat {
			n++
		}
	}
	return n
}

func TestExampleIsClean(t *testing.T) {
	const path = "../../examples/example.modelith.yaml"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(path, data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("expected example to lint clean, got %d findings: %+v", len(res.Findings), res.Findings)
	}
}

func TestStructuralErrors(t *testing.T) {
	cases := map[string]string{
		"missing kind/version": `entities: {}`,
		"bad cardinality": `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A thing.
    relationships:
      - entity: User
        cardinality: many
`,
		"entity missing definition": `
kind: DomainModel
version: v1
entities:
  Project: {}
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			res, err := Run(testModelPath, []byte(src), nil)
			if err != nil {
				t.Fatal(err)
			}
			if countBy(res.Findings, SeverityError, CategoryStructural) == 0 {
				t.Fatalf("expected a structural error, got: %+v", res.Findings)
			}
			if !res.HasBlocking(false) {
				t.Fatal("structural errors should block")
			}
		})
	}
}

func TestMalformedYAMLIsStructuralError(t *testing.T) {
	// Unbalanced brackets — not parseable as YAML at all.
	src := "kind: DomainModel\nentities: {Project: [unterminated\n"
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if countBy(res.Findings, SeverityError, CategoryStructural) == 0 {
		t.Fatalf("expected a structural error for malformed YAML, got: %+v", res.Findings)
	}
	var got string
	for _, f := range res.Findings {
		if strings.Contains(f.Message, "not valid YAML") {
			got = f.Message
		}
	}
	if got == "" {
		t.Fatalf("expected a 'not valid YAML' message, got: %+v", res.Findings)
	}
}

func TestNonObjectDocumentIsStructuralError(t *testing.T) {
	// A bare scalar is valid YAML/JSON but not an object: the schema rejects it
	// and lint should still produce a blocking structural finding rather than
	// proceeding to typed parsing.
	res, err := Run(testModelPath, []byte(`"just a string"`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if countBy(res.Findings, SeverityError, CategoryStructural) == 0 {
		t.Fatalf("expected a structural error for a non-object document, got: %+v", res.Findings)
	}
}

func TestUnsupportedVersionIsFriendlyError(t *testing.T) {
	src := `
kind: DomainModel
version: v2
entities:
  Project:
    definition: A container.
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if countBy(res.Findings, SeverityError, CategoryStructural) == 0 {
		t.Fatalf("expected a structural error for unsupported version, got: %+v", res.Findings)
	}
	var got string
	for _, f := range res.Findings {
		if f.Path == "/version" {
			got = f.Message
		}
	}
	if !strings.Contains(got, "unsupported schema version") || !strings.Contains(got, "v1") {
		t.Fatalf("expected a friendly unsupported-version message naming the supported versions, got: %q", got)
	}
}

// TestRunStructural_UnsupportedVersionReturnsEntityScopes locks down the named
// return runStructural hands back on the unsupported-version early-return
// path (lint.go:195). That line used to declare a new, block-scoped
// entityScopes with `:=`, shadowing the function's named return instead of
// assigning it; the explicit `return false, entityScopes` right after still
// returned the correct (shadowed) value, so the bug was latent, not live —
// but it would have gone silently wrong the moment that return became a bare
// `return`, which is exactly the idiom named returns invite. Calling
// runStructural directly (white-box, same package) is the only way to
// observe this return value: Run() never reaches this path with structuralOK
// true, so nothing downstream currently consumes it.
func TestRunStructural_UnsupportedVersionReturnsEntityScopes(t *testing.T) {
	src := `
kind: DomainModel
version: v99
entities:
  Ticket:
    definition: A parking ticket.
    subtypeOf: payments.Invoice
`
	res := &Result{}
	ok, entityScopes := runStructural([]byte(src), res)
	if ok {
		t.Fatal("expected ok=false for an unsupported version")
	}
	want := map[string]bool{"payments": true}
	if len(entityScopes) != len(want) || !entityScopes["payments"] {
		t.Fatalf("expected entityScopes %+v from the entity-position reference, got %+v", want, entityScopes)
	}
}

func TestUndefinedRelationshipTargetIsError(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A container.
    relationships:
      - entity: Ghost
        cardinality: "1:n"
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if countBy(res.Findings, SeverityError, CategorySemantic) == 0 {
		t.Fatalf("expected semantic error for undefined target, got: %+v", res.Findings)
	}
}

func TestUnknownBacktickTermIsWarning(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A container that mentions a ` + "`Phantom`" + ` concept.
    invariants:
      - id: always-has-something
        statement: "Always has something"
scenarios:
  - name: touch project
    steps:
      - "Use the ` + "`Project`" + `"
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if countBy(res.Findings, SeverityWarning, CategorySemantic) == 0 {
		t.Fatalf("expected a semantic warning for unknown term, got: %+v", res.Findings)
	}
	if res.HasBlocking(false) {
		t.Fatal("a lone semantic warning should not block")
	}
}

func findingWithMessage(fs []Finding, substr string) bool {
	for _, f := range fs {
		if strings.Contains(f.Message, substr) {
			return true
		}
	}
	return false
}

// An entity that declares invariants but is exercised by no scenario should
// trigger the "no scenario exercises" completeness path and NOT the
// "has no invariants" one.
func TestEntityWithInvariantsButNoScenario(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Widget:
    definition: A thing with a rule that nothing exercises.
    invariants:
      - id: always-valid
        statement: "Always valid"
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !findingWithMessage(res.Findings, "no scenario exercises") {
		t.Fatalf("expected a 'no scenario exercises' completeness warning, got: %+v", res.Findings)
	}
	if findingWithMessage(res.Findings, "has no invariants") {
		t.Fatalf("did not expect a 'has no invariants' warning (Widget has one): %+v", res.Findings)
	}
}

// An entity named only as a scenario actor (never in a step) still counts as
// exercised — the actor-marking branch of completeness.
func TestActorMarksEntityExercised(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A container.
    invariants:
      - id: has-an-owner
        statement: "Has an owner"
scenarios:
  - name: do something
    actors: [Project]
    steps:
      - "something happens"
    invariants_touched:
      - has-an-owner
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if findingWithMessage(res.Findings, "no scenario exercises") {
		t.Fatalf("Project is a scenario actor and should count as exercised: %+v", res.Findings)
	}
}

func TestHasBlockingTable(t *testing.T) {
	errOnly := &Result{Findings: []Finding{{Severity: SeverityError, Category: CategorySemantic}}}
	warnOnly := &Result{Findings: []Finding{{Severity: SeverityWarning, Category: CategorySemantic}}}
	complOnly := &Result{Findings: []Finding{{Severity: SeverityWarning, Category: CategoryCompleteness}}}
	cases := []struct {
		name              string
		res               *Result
		completenessAsErr bool
		want              bool
	}{
		{"error blocks in warn mode", errOnly, false, true},
		{"error blocks in error mode", errOnly, true, true},
		{"semantic warning never blocks", warnOnly, false, false},
		{"semantic warning never blocks (error mode)", warnOnly, true, false},
		{"completeness does not block in warn mode", complOnly, false, false},
		{"completeness blocks in error mode", complOnly, true, true},
		{"empty result never blocks", &Result{}, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.res.HasBlocking(c.completenessAsErr); got != c.want {
				t.Errorf("HasBlocking(%v) = %v, want %v", c.completenessAsErr, got, c.want)
			}
		})
	}
}

// A relationship declared from both sides with cardinalities that aren't
// inverses is a contradiction and must be a hard error.
func TestReciprocalCardinalityConflictIsError(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A container.
    relationships:
      - entity: Policy
        cardinality: "1:n"
  Policy:
    definition: A rule.
    relationships:
      - entity: Project
        cardinality: "1:1"
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !findingWithMessage(res.Findings, "reciprocal cardinality conflict") {
		t.Fatalf("expected a reciprocal cardinality conflict error, got: %+v", res.Findings)
	}
	if !res.HasBlocking(false) {
		t.Fatal("a reciprocal cardinality conflict should block")
	}
}

// Inverse cardinalities (1:n one way, n:1 the other) describe the same
// relationship from both ends and must NOT be flagged.
func TestReciprocalInverseCardinalityIsClean(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A container.
    relationships:
      - entity: Policy
        cardinality: "1:n"
  Policy:
    definition: A rule.
    relationships:
      - entity: Project
        cardinality: "n:1"
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if findingWithMessage(res.Findings, "reciprocal cardinality conflict") {
		t.Fatalf("inverse cardinalities should be clean, got: %+v", res.Findings)
	}
}

// When a pair has multiple declarations in one direction the edges can't be
// paired up unambiguously, so the reciprocity check is skipped rather than
// guessing — even if a naive comparison would flag a conflict.
func TestReciprocalMultipleEdgesSkipped(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A container.
    relationships:
      - entity: User
        cardinality: "n:n"
        role: "` + "`Owner`" + `"
      - entity: User
        cardinality: "n:1"
        role: "` + "`Member`" + `"
  User:
    definition: A person.
    relationships:
      - entity: Project
        cardinality: "1:1"
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if findingWithMessage(res.Findings, "reciprocal cardinality conflict") {
		t.Fatalf("multiple edges per direction should skip the check, got: %+v", res.Findings)
	}
}

func TestCompletenessAndPromotion(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Lonely:
    definition: An entity with no invariants and no scenario.
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if countBy(res.Findings, SeverityWarning, CategoryCompleteness) < 2 {
		t.Fatalf("expected completeness warnings (no invariants, no scenario), got: %+v", res.Findings)
	}
	if res.HasBlocking(false) {
		t.Fatal("completeness warnings should not block by default")
	}
	if !res.HasBlocking(true) {
		t.Fatal("completeness warnings should block when promoted to error")
	}
}

// --- Cluster C: glossary, enums, invariant ids, structured actions ---

func TestUnknownInvariantTouchedIsError(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A container.
    invariants:
      - id: real-rule
        statement: "Has a rule"
scenarios:
  - name: do it
    steps: ["the ` + "`Project`" + ` does a thing"]
    invariants_touched: [no-such-rule]
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !findingWithMessage(res.Findings, "unknown invariant id \"no-such-rule\"") {
		t.Fatalf("expected an error for the dangling invariant id, got: %+v", res.Findings)
	}
	if !res.HasBlocking(false) {
		t.Fatal("a dangling invariant reference should block")
	}
}

func TestDuplicateInvariantIDIsError(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A container.
    invariants:
      - id: dup
        statement: "First"
  User:
    definition: A principal.
    invariants:
      - id: dup
        statement: "Second"
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !findingWithMessage(res.Findings, "duplicate invariant id \"dup\"") {
		t.Fatalf("expected a duplicate-id error, got: %+v", res.Findings)
	}
}

// A model-level invariant id colliding with an entity-level one is a duplicate
// across the shared id namespace and must be an error, regardless of scope.
func TestModelLevelInvariantDuplicateIDIsError(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A container.
    invariants:
      - id: dup
        statement: "Entity-level rule"
invariants:
  - id: dup
    statement: "Model-level rule for the ` + "`Project`" + `"
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !findingWithMessage(res.Findings, "duplicate invariant id \"dup\"") {
		t.Fatalf("expected a cross-scope duplicate-id error, got: %+v", res.Findings)
	}
	if !res.HasBlocking(false) {
		t.Fatal("a duplicate invariant id should block")
	}
}

// A scenario's invariants_touched and an action's preserves must resolve against
// model-level invariants, not just entity-level ones.
func TestModelLevelInvariantResolvesAcrossScopes(t *testing.T) {
	src := `
kind: DomainModel
version: v1
glossary:
  Owner: "An owner."
entities:
  Project:
    definition: A container exercised below.
    actions:
      - name: archive
        actor: Owner
        preserves: [cross-entity-rule]
invariants:
  - id: cross-entity-rule
    statement: "A rule spanning the ` + "`Project`" + ` and beyond"
scenarios:
  - name: use it
    steps: ["the ` + "`Project`" + ` is archived"]
    invariants_touched: [cross-entity-rule]
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if findingWithMessage(res.Findings, "unknown invariant id") {
		t.Fatalf("references to a model-level invariant should resolve, got: %+v", res.Findings)
	}
	if res.HasBlocking(false) {
		t.Fatalf("model-level invariant references should not block, got: %+v", res.Findings)
	}
}

// A reference to a model-level invariant id that doesn't exist is still a
// dangling reference and must error.
func TestDanglingReferenceWithModelLevelInvariantsIsError(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A container.
invariants:
  - id: real-model-rule
    statement: "A real rule for the ` + "`Project`" + `"
scenarios:
  - name: use it
    steps: ["the ` + "`Project`" + ` is used"]
    invariants_touched: [ghost-model-rule]
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !findingWithMessage(res.Findings, "unknown invariant id \"ghost-model-rule\"") {
		t.Fatalf("expected a dangling-reference error, got: %+v", res.Findings)
	}
	if !res.HasBlocking(false) {
		t.Fatal("a dangling invariant reference should block")
	}
}

func TestUndefinedRoleIsWarning(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A container.
    relationships:
      - entity: User
        cardinality: "n:n"
        role: "` + "`Maintainer`" + `"
  User:
    definition: A principal.
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !findingWithMessage(res.Findings, "role term \"Maintainer\" is not a defined entity or glossary term") {
		t.Fatalf("expected a warning for the undefined role, got: %+v", res.Findings)
	}
	if res.HasBlocking(false) {
		t.Fatal("an undefined role is advisory, not blocking")
	}
}

func TestGlossaryRoleResolvesCleanly(t *testing.T) {
	src := `
kind: DomainModel
version: v1
glossary:
  Maintainer: "A ` + "`User`" + ` who maintains a ` + "`Project`" + `."
entities:
  Project:
    definition: A container exercised by the scenario.
    relationships:
      - entity: User
        cardinality: "n:n"
        role: "` + "`Maintainer`" + `"
  User:
    definition: A principal.
scenarios:
  - name: maintain
    actors: [Maintainer]
    steps: ["a ` + "`Maintainer`" + ` touches the ` + "`Project`" + ` and the ` + "`User`" + `"]
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if findingWithMessage(res.Findings, "Maintainer") {
		t.Fatalf("a glossary-defined role should not warn, got: %+v", res.Findings)
	}
}

// A prose role is the only label on the rendered relationship line (ADR-0008),
// so it collides with its neighbours in the diagram. The linter steers the
// prose to `note` — as a warning, never an error.
func TestADR_0008_ProseRoleIsWarning(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		role string
		want bool
	}{
		{name: "role name", role: "`Owner`", want: false},
		{name: "two role names", role: "`Owner` or `Member`", want: false},
		{name: "a comma-separated pair of role names is a list, not prose", role: "`Owner`, `Member`", want: false},
		{name: "a field accessor is not a sentence", role: "`Project`.owner", want: false},
		{name: "a version number is not a sentence", role: "v1.0 owner", want: false},
		{name: "four words", role: "primary contact for escalation", want: false},
		{name: "five words", role: "the record this one supersedes", want: true},
		{name: "one word too long for a label", role: "pneumonoultramicroscopicsilicovolcanoconiosisadministrator", want: true},
		{name: "full stop", role: "The owning project.", want: true},
		{name: "semicolon", role: "owner; also billing", want: true},
		{name: "empty", role: "", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A container.
    relationships:
      - entity: User
        cardinality: "n:n"
        role: ` + strconv.Quote(tc.role) + `
  User:
    definition: A principal.
`
			res, err := Run(testModelPath, []byte(src), nil)
			if err != nil {
				t.Fatal(err)
			}
			got := findingWithMessage(res.Findings, "reads as prose")
			if got != tc.want {
				t.Fatalf("role %q: prose warning = %v, want %v; findings: %+v", tc.role, got, tc.want, res.Findings)
			}
			if !got {
				return
			}
			for _, f := range res.Findings {
				if !strings.Contains(f.Message, "reads as prose") {
					continue
				}
				if f.Severity != SeverityWarning || f.Category != CategorySemantic {
					t.Errorf("expected a semantic warning, got %s/%s", f.Severity, f.Category)
				}
				if want := "/entities/Project/relationships/0/role"; f.Path != want {
					t.Errorf("path = %q, want %q", f.Path, want)
				}
			}
			if res.HasBlocking(false) {
				t.Error("a prose role is advisory, not blocking")
			}
		})
	}
}

// TestProseRoleRaisesOneFinding pins that a role produces at most one finding.
// A prose role that also buries an undefined backticked term used to raise both
// warnings at the same path; rewriting the role is the fix that comes first and
// will likely change the term with it.
func TestProseRoleRaisesOneFinding(t *testing.T) {
	t.Parallel()
	src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A container.
    relationships:
      - entity: User
        cardinality: "n:n"
        role: "the ` + "`Widget`" + ` this one supersedes"
  User:
    definition: A principal.
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	var atRole []Finding
	for _, f := range res.Findings {
		if f.Path == "/entities/Project/relationships/0/role" {
			atRole = append(atRole, f)
		}
	}
	if len(atRole) != 1 {
		t.Fatalf("expected exactly one finding on the role, got %d: %+v", len(atRole), atRole)
	}
	if !strings.Contains(atRole[0].Message, "reads as prose") {
		t.Errorf("expected the prose finding to be the one kept, got %q", atRole[0].Message)
	}
}

// TestADR_0008_MutualOwnershipIsError pins the one ownership contradiction a
// reciprocal pair can hold: both ends claiming to own the other. A relationship
// is owned by at most one end, so the renderer refuses to fold it and the
// linter names it. Exactly one end owning is the ordinary composition pattern
// and must stay clean whatever roles the two ends give it — see
// TestADR_0008_ReciprocalCompositionLintsClean.
func TestADR_0008_MutualOwnershipIsError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		aOwn, bOwn string
		aRole      string
		bRole      string
		want       string // "" means no ownership finding
	}{
		{
			name: "mutual owned is a contradiction",
			aOwn: "owned", bOwn: "owned",
			want: "mutual ownership",
		},
		{
			name: "mutual owned under differing roles is still a contradiction",
			aOwn: "owned", bOwn: "owned",
			aRole: "part", bRole: "whole",
			want: "mutual ownership",
		},
		{
			name: "one end owning, the other referencing",
			aOwn: "owned", bOwn: "referenced",
			aRole: "part", bRole: "whole",
			want: "",
		},
		{
			name: "an omitted ownership on the other end is the same as referenced",
			aOwn: "owned", bOwn: "",
			want: "",
		},
		{
			name: "neither end claims ownership",
			aOwn: "", bOwn: "",
			aRole: "part", bRole: "whole",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			field := func(name, val string) string {
				if val == "" {
					return ""
				}
				return "\n        " + name + ": " + strconv.Quote(val)
			}
			src := `
kind: DomainModel
version: v1
glossary:
  Part: "A component."
entities:
  Alpha:
    definition: A container.
    relationships:
      - entity: Beta
        cardinality: "1:n"` + field("ownership", tc.aOwn) + field("role", tc.aRole) + `
  Beta:
    definition: A component.
    relationships:
      - entity: Alpha
        cardinality: "n:1"` + field("ownership", tc.bOwn) + field("role", tc.bRole) + `
`
			res, err := Run(testModelPath, []byte(src), nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := findingWithMessage(res.Findings, "mutual ownership"); got != (tc.want != "") {
				t.Fatalf("mutual-ownership finding = %v, want %v; findings: %+v", got, tc.want != "", res.Findings)
			}
			if tc.want == "" {
				if res.HasBlocking(false) {
					t.Fatalf("expected no error on a legitimate pair, got: %+v", res.Findings)
				}
				return
			}
			for _, f := range res.Findings {
				if !strings.Contains(f.Message, tc.want) {
					continue
				}
				if f.Severity != SeverityError || f.Category != CategorySemantic {
					t.Errorf("expected a semantic error, got %s/%s", f.Severity, f.Category)
				}
				if want := "/entities/Alpha/relationships/0/ownership"; f.Path != want {
					t.Errorf("path = %q, want %q", f.Path, want)
				}
			}
			if !res.HasBlocking(false) {
				t.Error("an ownership contradiction must block")
			}
		})
	}
}

// TestADR_0008_ReciprocalCompositionLintsClean is the linter half of the
// regression guard on the textbook composition pattern: a parent owns a child,
// the child references the parent back, and each end names its own role. It is
// one relationship seen from two sides, which the renderer folds into a single
// solid line (TestADR_0008_ReciprocalCompositionFoldsToOneEdge), so it must
// produce no finding at all — not a warning, and certainly not an error that
// would fail a CI gate on a correct model.
func TestADR_0008_ReciprocalCompositionLintsClean(t *testing.T) {
	t.Parallel()
	src := `
kind: DomainModel
version: v1
title: Composition declared from both ends
entities:
  Alpha:
    definition: A container.
    invariants:
      - id: alpha-1
        statement: An Alpha always has a name.
    relationships:
      - entity: Beta
        cardinality: "1:n"
        ownership: owned
        role: part
  Beta:
    definition: A component.
    invariants:
      - id: beta-1
        statement: A Beta always belongs to an Alpha.
    relationships:
      - entity: Alpha
        cardinality: "n:1"
        ownership: referenced
        role: whole
scenarios:
  - name: compose
    actors: [Alpha]
    steps: ["an ` + "`Alpha`" + ` gains a ` + "`Beta`" + `"]
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("expected no findings on a reciprocal composition, got %d: %+v", len(res.Findings), res.Findings)
	}
}

// TestADR_0008_AmbiguousPairingIsWarning pins the diagnostic for a pair whose
// reciprocal declarations cannot be matched up: one end declares the same line
// twice and the other declares it once, so which is the reciprocal of which is
// undeterminable. The renderer refuses to guess and draws all of them, which is
// lossless but shows more lines than the author probably means.
//
// A warning, never an error: the model may be exactly right. It must stay
// silent whenever each direction declares the line at most once, which is every
// shipped example and every other fixture.
func TestADR_0008_AmbiguousPairingIsWarning(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		rels string
		want bool
		path string
	}{
		{
			name: "two forward, one back",
			rels: `
  Project:
    definition: A container.
    relationships:
      - entity: Policy
        cardinality: "1:n"
        role: defaults
      - entity: Policy
        cardinality: "1:n"
        role: overrides
  Policy:
    definition: A rule.
    relationships:
      - entity: Project
        cardinality: "n:1"
        role: parent
        ownership: owned
`,
			want: true,
			path: "/entities/Project/relationships/0",
		},
		{
			name: "one forward, two back",
			rels: `
  Project:
    definition: A container.
    relationships:
      - entity: Policy
        cardinality: "1:n"
        role: parent
        ownership: owned
  Policy:
    definition: A rule.
    relationships:
      - entity: Project
        cardinality: "n:1"
        role: defaults
      - entity: Project
        cardinality: "n:1"
        role: overrides
`,
			want: true,
			path: "/entities/Policy/relationships/0",
		},
		{
			name: "two forward, none back: nothing to pair, so nothing ambiguous",
			rels: `
  Project:
    definition: A container.
    relationships:
      - entity: Policy
        cardinality: "1:n"
        role: defaults
      - entity: Policy
        cardinality: "1:n"
        role: overrides
  Policy:
    definition: A rule.
`,
			want: false,
		},
		{
			name: "one each way: the ordinary reciprocal",
			rels: `
  Project:
    definition: A container.
    relationships:
      - entity: Policy
        cardinality: "1:n"
        role: parent
        ownership: owned
  Policy:
    definition: A rule.
    relationships:
      - entity: Project
        cardinality: "n:1"
        role: child
`,
			want: false,
		},
		{
			name: "two forward at different cardinalities, one back: distinct lines",
			rels: `
  Project:
    definition: A container.
    relationships:
      - entity: Policy
        cardinality: "1:n"
        role: defaults
      - entity: Policy
        cardinality: "1:1"
        role: primary
  Policy:
    definition: A rule.
    relationships:
      - entity: Project
        cardinality: "n:1"
        role: parent
        ownership: owned
`,
			want: false,
		},
		{
			name: "the second forward declaration is an exact duplicate of the first",
			rels: `
  Project:
    definition: A container.
    relationships:
      - entity: Policy
        cardinality: "1:n"
        role: defaults
      - entity: Policy
        cardinality: "1:n"
        role: defaults
  Policy:
    definition: A rule.
    relationships:
      - entity: Project
        cardinality: "n:1"
        role: parent
        ownership: owned
`,
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := Run(testModelPath, []byte("kind: DomainModel\nversion: v1\nentities:"+tc.rels), nil)
			if err != nil {
				t.Fatal(err)
			}
			got := findingWithMessage(res.Findings, "ambiguous reciprocal pairing")
			if got != tc.want {
				t.Fatalf("ambiguous-pairing warning = %v, want %v; findings: %+v", got, tc.want, res.Findings)
			}
			if !got {
				return
			}
			for _, f := range res.Findings {
				if !strings.Contains(f.Message, "ambiguous reciprocal pairing") {
					continue
				}
				if f.Severity != SeverityWarning || f.Category != CategorySemantic {
					t.Errorf("expected a semantic warning, got %s/%s", f.Severity, f.Category)
				}
				if f.Path != tc.path {
					t.Errorf("path = %q, want %q", f.Path, tc.path)
				}
			}
			if res.HasBlocking(false) {
				t.Error("an ambiguous pairing is advisory, not blocking")
			}
		})
	}
}

func TestActionPreservesUnknownInvariantIsError(t *testing.T) {
	src := `
kind: DomainModel
version: v1
glossary:
  Owner: "An owner."
entities:
  Project:
    definition: A container.
    actions:
      - name: archive
        actor: Owner
        preserves: [ghost-rule]
    invariants:
      - id: real-rule
        statement: "Has a rule"
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !findingWithMessage(res.Findings, "preserves unknown invariant id \"ghost-rule\"") {
		t.Fatalf("expected an error for preserves of an unknown id, got: %+v", res.Findings)
	}
}

func TestEnumTypeMustResolve(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A container.
    attributes:
      - name: status
        type: ProjectStatus
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !findingWithMessage(res.Findings, "looks like an enum reference but no enum \"ProjectStatus\" is defined") {
		t.Fatalf("expected a warning for the unresolved enum type, got: %+v", res.Findings)
	}
}

func TestUnusedGlossaryAndEnumAreAdvisory(t *testing.T) {
	src := `
kind: DomainModel
version: v1
glossary:
  Ghost: "Never referenced."
enums:
  Unused:
    values:
      - name: x
entities:
  Project:
    definition: A container.
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !findingWithMessage(res.Findings, "glossary term \"Ghost\" is defined but never referenced") {
		t.Fatalf("expected an unused-glossary advisory, got: %+v", res.Findings)
	}
	if !findingWithMessage(res.Findings, "enum \"Unused\" is defined but no attribute uses it") {
		t.Fatalf("expected an unused-enum advisory, got: %+v", res.Findings)
	}
	if countBy(res.Findings, SeverityError, CategoryCompleteness) != 0 {
		t.Fatalf("completeness advisories should be warnings, not errors: %+v", res.Findings)
	}
}

// TestCompleteness_SharedRelaxesOnlyTheUnusedChecks pins the escape hatch a
// vocabulary model needs. A model that exists to be imported has its enums and
// glossary terms referenced from files it cannot see, so under
// --completeness error it could not pass at all; `shared: true` says the
// references are elsewhere. It says nothing about content, so a gap that being
// imported does not fill — an entity with no invariants, an entity no scenario
// exercises — is still reported.
func TestCompleteness_SharedRelaxesOnlyTheUnusedChecks(t *testing.T) {
	t.Parallel()

	// A vocabulary model with nothing missing from it: every entity has an
	// invariant and a scenario that exercises it. All that is left is the enum
	// and the glossary term it exists to publish, which nothing here uses.
	const vocabulary = `
kind: DomainModel
version: v1
glossary:
  Payer: Whoever hands over the money; named only by the models that import this one.
enums:
  PaymentMethod:
    values:
      - name: card
scenarios:
  - name: A charge is settled
    actors: [Payment]
    steps:
      - A card is tapped and the transfer is recorded.
    invariants_touched: [payment-amount-positive]
entities:
  Payment:
    definition: One completed transfer of money.
    invariants:
      - id: payment-amount-positive
        statement: A payment's amount is greater than zero.
`
	// A gap a shared model does not get to skip: an entity nothing governs and
	// no scenario exercises is missing from the model itself, which being
	// imported does not fill.
	const contentGap = "  Refund:\n    definition: Money returned.\n"

	cases := []struct {
		name string
		src  string
		// unused is whether the defined-but-locally-unused findings are expected.
		unused bool
		// blocking is HasBlocking under --completeness error.
		blocking bool
		gap      bool
	}{
		{name: "not shared", src: vocabulary, unused: true, blocking: true},
		{
			name: "shared",
			src:  strings.Replace(vocabulary, "version: v1", "version: v1\nshared: true", 1),
		},
		{
			name:     "shared, with a gap that is not about being used",
			src:      strings.Replace(vocabulary, "version: v1", "version: v1\nshared: true", 1) + contentGap,
			blocking: true,
			gap:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := Run(testModelPath, []byte(tc.src), nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, msg := range []string{
				`glossary term "Payer" is defined but never referenced`,
				`enum "PaymentMethod" is defined but no attribute uses it`,
			} {
				if got := findingWithMessage(res.Findings, msg); got != tc.unused {
					t.Errorf("finding %q present = %v, want %v: %+v", msg, got, tc.unused, res.Findings)
				}
			}
			for _, msg := range []string{
				`entity "Refund" has no invariants`,
				`no scenario exercises entity "Refund"`,
			} {
				if got := findingWithMessage(res.Findings, msg); got != tc.gap {
					t.Errorf("finding %q present = %v, want %v: %+v", msg, got, tc.gap, res.Findings)
				}
			}
			// The boundary this exists for: only the shared model with nothing
			// else missing passes --completeness error.
			if got := res.HasBlocking(true); got != tc.blocking {
				t.Errorf("HasBlocking(true) = %v, want %v: %+v", got, tc.blocking, res.Findings)
			}
		})
	}
}

func TestDerivedRequiresDerivation(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A container.
    attributes:
      - name: count
        type: integer
        derived: true
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if countBy(res.Findings, SeverityError, CategoryStructural) == 0 {
		t.Fatalf("expected a structural error: derived attribute without derivation, got: %+v", res.Findings)
	}
}

func TestDerivationWithoutDerivedIsRejected(t *testing.T) {
	// An orphaned `derivation` on a non-derived attribute is a structural error
	// (schema if/then/else): derivation only belongs on a derived attribute.
	src := `
kind: DomainModel
version: v1
entities:
  Project:
    definition: A container.
    attributes:
      - name: count
        type: integer
        derivation: "counts something"
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if countBy(res.Findings, SeverityError, CategoryStructural) == 0 {
		t.Fatalf("expected a structural error: derivation without derived, got: %+v", res.Findings)
	}
}

func TestEntityDerived_DerivationOptional(t *testing.T) {
	// Unlike the attribute feature, an entity's derivation is optional even
	// when derived is true — the definition often already explains it.
	src := `
kind: DomainModel
version: v1
entities:
  Report:
    definition: A computed summary. No derivation string given.
    derived: true
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if countBy(res.Findings, SeverityError, CategoryStructural) != 0 {
		t.Fatalf("expected derived entity without derivation to lint clean structurally, got: %+v", res.Findings)
	}
}

func TestEntityDerivationWithoutDerivedIsRejected(t *testing.T) {
	// An orphaned entity `derivation` without `derived: true` is a structural
	// error (schema if/then/else), mirroring the attribute rule.
	src := `
kind: DomainModel
version: v1
entities:
  Report:
    definition: A computed summary.
    derivation: "Computed from other state."
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if countBy(res.Findings, SeverityError, CategoryStructural) == 0 {
		t.Fatalf("expected a structural error: entity derivation without derived, got: %+v", res.Findings)
	}
}

func TestDerivedEntity_OwnedTargetWarning(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Scene:
    definition: A resolved editable canvas.
    relationships:
      - entity: Diagnostic
        cardinality: "1:n"
        ownership: owned
  Diagnostic:
    definition: A finding computed from resolved scene geometry.
    derived: true
    derivation: Recomputed on every query from the resolved geometry.
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := Finding{
		Severity: SeverityWarning,
		Category: CategorySemantic,
		Path:     "/entities/Scene/relationships/0/ownership",
		Message:  `entity "Scene" owns "Diagnostic", which is derived — composing an ephemeral, never-persisted entity is usually a modeling error`,
	}
	found := false
	for _, f := range res.Findings {
		if f == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected finding %+v, got: %+v", want, res.Findings)
	}
	if res.HasBlocking(false) {
		t.Fatal("a lone derived-ownership warning should not block")
	}
}

func TestDerivedEntity_ReferencedTargetIsClean(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Scene:
    definition: A resolved editable canvas.
    relationships:
      - entity: Diagnostic
        cardinality: "1:n"
        ownership: referenced
  Diagnostic:
    definition: A finding computed from resolved scene geometry.
    derived: true
    derivation: Recomputed on every query from the resolved geometry.
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if findingWithMessage(res.Findings, "which is derived") {
		t.Fatalf("did not expect a derived-ownership warning for a referenced relationship, got: %+v", res.Findings)
	}
}

func TestBareAndStructuredActionsCoexist(t *testing.T) {
	src := `
kind: DomainModel
version: v1
glossary:
  Owner: "An owner."
entities:
  Project:
    definition: A container exercised below.
    actions:
      - create
      - name: archive
        actor: Owner
        preserves: [rule]
    invariants:
      - id: rule
        statement: "A rule"
scenarios:
  - name: use it
    steps: ["the ` + "`Project`" + ` is used"]
    invariants_touched: [rule]
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.HasBlocking(false) {
		t.Fatalf("mixed bare/structured actions should be valid, got: %+v", res.Findings)
	}
}

// TestADR_0003_SymmetricRequiresInterchangeableEnds pins the symmetric-marker
// rule from ADR-0003: symmetric is only meaningful on a self-referential
// relationship or one whose target side is more than one.
func TestADR_0003_SymmetricRequiresInterchangeableEnds(t *testing.T) {
	// Valid: symmetric self-referential; symmetric on an exact pair.
	valid := `
kind: DomainModel
version: v1
entities:
  Node:
    definition: A node that peers with other nodes.
    relationships:
      - entity: Node
        cardinality: "n:n"
        symmetric: true
        role: peers with
  Pair:
    definition: An unordered pair of nodes.
    relationships:
      - entity: Node
        cardinality: "1:2"
        symmetric: true
        role: the unordered pair
`
	res, err := Run(testModelPath, []byte(valid), nil)
	if err != nil {
		t.Fatal(err)
	}
	if findingWithMessage(res.Findings, "must be self-referential") {
		t.Fatalf("valid symmetric relationships should not be flagged: %+v", res.Findings)
	}

	// Invalid: symmetric on a directed 1:1 to a different entity.
	invalid := `
kind: DomainModel
version: v1
entities:
  Passport:
    definition: A passport held by exactly one person.
    relationships:
      - entity: Person
        cardinality: "1:1"
        symmetric: true
  Person:
    definition: A person.
`
	res, err = Run(testModelPath, []byte(invalid), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !findingWithMessage(res.Findings, "must be self-referential") {
		t.Fatalf("expected a symmetric-misuse error, got: %+v", res.Findings)
	}
}

// TestADR_0003_InvalidCardinalityRange pins that an inverted range like "5..2",
// which the schema pattern accepts, is caught as a semantic error.
func TestADR_0003_InvalidCardinalityRange(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  A:
    definition: An entity with a broken cardinality range.
    relationships:
      - entity: B
        cardinality: "1:5..2"
  B:
    definition: Another entity.
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !findingWithMessage(res.Findings, "minimum must not exceed its maximum") {
		t.Fatalf("expected an invalid-range error, got: %+v", res.Findings)
	}
}

// TestReciprocitySemanticEquality guards the review fix: "1:n" and "0..n:1" are
// the same relationship inverted (n == 0..n), so declaring both sides that way
// must NOT read as a conflict.
func TestReciprocitySemanticEquality(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Alpha:
    definition: Owns many betas.
    relationships:
      - entity: Beta
        cardinality: "1:n"
        role: owns
  Beta:
    definition: Belongs to one alpha.
    relationships:
      - entity: Alpha
        cardinality: "0..n:1"
        role: owns
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if findingWithMessage(res.Findings, "reciprocal cardinality conflict") {
		t.Fatalf("semantically equal inverses should not conflict: %+v", res.Findings)
	}
}

// TestSymmetricOnInvalidRangeNoDoubleError guards the review fix: an inverted
// range with symmetric: true reports the range error only, not a second
// confusing symmetric-misuse error.
func TestSymmetricOnInvalidRangeNoDoubleError(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  A:
    definition: Broken range, symmetric set.
    relationships:
      - entity: B
        cardinality: "1:5..2"
        symmetric: true
  B:
    definition: Another.
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !findingWithMessage(res.Findings, "minimum must not exceed its maximum") {
		t.Fatalf("expected the range error: %+v", res.Findings)
	}
	if findingWithMessage(res.Findings, "must be self-referential") {
		t.Fatalf("should not also emit the symmetric-misuse error: %+v", res.Findings)
	}
}

// TestADR_0004_UndefinedSupertype pins that a subtypeOf naming no defined
// entity is a semantic error.
func TestADR_0004_UndefinedSupertype(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  Card:
    definition: A card that claims to be a kind of an undefined thing.
    subtypeOf: PaymentMethod
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !findingWithMessage(res.Findings, "subtype of undefined entity") {
		t.Fatalf("expected an undefined-supertype error, got: %+v", res.Findings)
	}
}

// TestADR_0004_SubtypeInheritsInvariants pins that a supertype's invariants
// cover a subtype for completeness: a subtype with no invariants of its own is
// not flagged when its parent has them, while an unrelated empty entity is.
func TestADR_0004_SubtypeInheritsInvariants(t *testing.T) {
	src := `
kind: DomainModel
version: v1
entities:
  PaymentMethod:
    definition: A way to pay.
    invariants:
      - id: pm-usable
        statement: A ` + "`PaymentMethod`" + ` is either active or revoked.
  Card:
    definition: A payment method backed by a card; adds no rule of its own.
    subtypeOf: PaymentMethod
  Cash:
    definition: A standalone entity with no rule and no parent.
`
	res, err := Run(testModelPath, []byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	if findingWithMessage(res.Findings, `"Card" has no invariants`) {
		t.Fatalf("subtype should inherit its parent's invariants for completeness: %+v", res.Findings)
	}
	if !findingWithMessage(res.Findings, `"Cash" has no invariants`) {
		t.Fatalf("an unrelated empty entity should still be flagged: %+v", res.Findings)
	}
}

// TestSubtypeCycleDetected pins that a self- or mutually-cyclic subtypeOf chain
// is a semantic error rather than an infinite walk.
func TestSubtypeCycleDetected(t *testing.T) {
	self := `
kind: DomainModel
version: v1
entities:
  A:
    definition: An entity that is a kind of itself.
    subtypeOf: A
`
	res, err := Run(testModelPath, []byte(self), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !findingWithMessage(res.Findings, "cyclic subtypeOf chain") {
		t.Fatalf("expected a self-cycle error, got: %+v", res.Findings)
	}

	mutual := `
kind: DomainModel
version: v1
entities:
  Alpha:
    definition: A kind of Beta.
    subtypeOf: Beta
  Beta:
    definition: A kind of Alpha.
    subtypeOf: Alpha
`
	res, err = Run(testModelPath, []byte(mutual), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !findingWithMessage(res.Findings, "cyclic subtypeOf chain") {
		t.Fatalf("expected a mutual-cycle error, got: %+v", res.Findings)
	}
}
