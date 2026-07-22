package lint

import (
	"os"
	"strings"
	"testing"
)

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
	data, err := os.ReadFile("../../examples/example.modelith.yaml")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(data)
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
			res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(`"just a string"`))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if findingWithMessage(res.Findings, "Maintainer") {
		t.Fatalf("a glossary-defined role should not warn, got: %+v", res.Findings)
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if countBy(res.Findings, SeverityError, CategoryStructural) == 0 {
		t.Fatalf("expected a structural error: derivation without derived, got: %+v", res.Findings)
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(valid))
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
	res, err = Run([]byte(invalid))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
	res, err := Run([]byte(src))
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
