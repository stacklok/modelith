package lint

import (
	"strings"
	"testing"
)

func planned(t *testing.T, inputs []Input, files fakeFiles) []FileResult {
	t.Helper()
	results, err := Plan(inputs, files)
	if err != nil {
		t.Fatal(err)
	}
	return results
}

func editedVendored(t *testing.T) string {
	t.Helper()
	copy := stamp(t, gappy)
	edited := strings.Replace(copy, "One car's stay in the garage.", "One car's stay.", 1)
	if edited == copy {
		t.Fatal("fixture did not change the vendored copy")
	}
	return edited
}

func TestPlan_DirectVendoredChild(t *testing.T) {
	t.Parallel()

	const root = "models/root.modelith.yaml"
	const child = "models/child.modelith.yaml"
	childSource := editedVendored(t)
	files := fakeFiles{".git": "", root: importer([]string{`"./child.modelith.yaml"`}, "child.PaymentMethod"), child: childSource}

	results := planned(t, []Input{{Path: root, Source: []byte(files[root])}}, files)
	if len(results) != 2 || results[1].File != child {
		t.Fatalf("results = %+v, want root followed by %q", results, child)
	}
	if len(results[1].Findings) != 1 || results[1].Findings[0].Path != "" || !strings.Contains(results[1].Findings[0].Message, "deps update "+child) {
		t.Errorf("child provenance result = %+v", results[1])
	}
}

func TestPlan_OwnedIntermediaryReachesVendoredGrandchild(t *testing.T) {
	t.Parallel()

	const root = "models/root.modelith.yaml"
	const middle = "models/middle.modelith.yaml"
	const child = "models/child.modelith.yaml"
	childSource := editedVendored(t)
	files := fakeFiles{
		".git": "",
		root:   importer([]string{`{scope: middle, path: "./middle.modelith.yaml"}`}, "middle.PaymentMethod"),
		middle: importer([]string{`{scope: child, path: "./child.modelith.yaml"}`}, "child.PaymentMethod"),
		child:  childSource,
	}

	results := planned(t, []Input{{Path: root, Source: []byte(files[root])}}, files)
	if len(results) != 2 || results[1].File != child || len(results[1].Findings) != 1 {
		t.Fatalf("results = %+v, want root plus grandchild provenance finding", results)
	}
	if !strings.Contains(results[1].Findings[0].Message, "deps update "+child) {
		t.Errorf("grandchild = %+v, want remedy for %q", results[1], child)
	}
}

func TestPlan_SharedChildIsVerifiedOnce(t *testing.T) {
	t.Parallel()

	const root = "models/root.modelith.yaml"
	const child = "models/child.modelith.yaml"
	files := fakeFiles{
		".git": "",
		root: importer([]string{
			`{scope: first, path: "./child.modelith.yaml"}`,
			`{scope: second, path: "./child.modelith.yaml"}`,
		}, "first.PaymentMethod"),
		child: editedVendored(t),
	}

	results := planned(t, []Input{{Path: root, Source: []byte(files[root])}}, files)
	if len(results) != 2 || results[1].File != child || len(results[1].Findings) != 1 {
		t.Errorf("results = %+v, want one provenance result for shared child", results)
	}
}

func TestPlan_CycleTerminates(t *testing.T) {
	t.Parallel()

	const first = "models/first.modelith.yaml"
	const second = "models/second.modelith.yaml"
	secondSource := stamp(t, importer([]string{`{scope: first, path: "./first.modelith.yaml"}`}, "first.PaymentMethod"))
	secondSource = strings.Replace(secondSource, "PaymentMethod", "EditedMethod", 1)
	files := fakeFiles{
		".git": "",
		first:  importer([]string{`{scope: second, path: "./second.modelith.yaml"}`}, "second.PaymentMethod"),
		second: secondSource,
	}

	results := planned(t, []Input{{Path: first, Source: []byte(files[first])}}, files)
	if len(results) != 2 || results[0].File != first || results[1].File != second || len(results[1].Findings) != 1 {
		t.Errorf("results = %+v, want one root and one visited cycle member", results)
	}
}

func TestPlan_BrokenNestedEdgeIsSilent(t *testing.T) {
	t.Parallel()

	const root = "models/root.modelith.yaml"
	const middle = "models/middle.modelith.yaml"
	files := fakeFiles{
		".git": "",
		root:   importer([]string{`{scope: middle, path: "./middle.modelith.yaml"}`}, "middle.PaymentMethod"),
		middle: importer([]string{`{scope: missing, path: "./missing.modelith.yaml"}`}, "missing.PaymentMethod"),
	}

	results := planned(t, []Input{{Path: root, Source: []byte(files[root])}}, files)
	if len(results) != 1 || results[0].File != root {
		t.Errorf("results = %+v, want only root and no nested missing-import finding", results)
	}
}

func TestPlan_ExplicitRootDiscoveredThroughImportRunsFullLintOnce(t *testing.T) {
	t.Parallel()

	const root = "models/root.modelith.yaml"
	const child = "models/child.modelith.yaml"
	files := fakeFiles{
		".git": "",
		root:   importer([]string{`{scope: child, path: "./child.modelith.yaml"}`}, "child.PaymentMethod"),
		child:  gappy,
	}

	results := planned(t, []Input{{Path: root, Source: []byte(files[root])}, {Path: child, Source: []byte(files[child])}}, files)
	if len(results) != 2 || results[1].File != child {
		t.Fatalf("results = %+v, want explicit child once", results)
	}
	var completeness bool
	for _, finding := range results[1].Findings {
		if finding.Category == CategoryCompleteness {
			completeness = true
		}
	}
	if !completeness {
		t.Errorf("explicit child did not receive full lint: %+v", results[1])
	}
}

func TestPlan_ExplicitAliasesRunFullLintInArgumentOrder(t *testing.T) {
	t.Parallel()

	const first = "./models/root.modelith.yaml"
	const second = "models/root.modelith.yaml"
	results := planned(t, []Input{
		{Path: first, Source: []byte(gappy)},
		{Path: second, Source: []byte(gappy)},
	}, fakeFiles{".git": ""})

	if len(results) != 2 || results[0].File != first || results[1].File != second {
		t.Fatalf("results = %+v, want both explicit aliases in argument order", results)
	}
	for _, result := range results {
		var completeness bool
		for _, finding := range result.Findings {
			if finding.Category == CategoryCompleteness {
				completeness = true
			}
		}
		if !completeness {
			t.Errorf("%s did not receive full lint: %+v", result.File, result.Findings)
		}
	}
}

func TestPlan_StructurallyInvalidExplicitRootDoesNotSeedTraversal(t *testing.T) {
	t.Parallel()

	const root = "models/root.modelith.yaml"
	const child = "models/child.modelith.yaml"
	rootSource := importer([]string{`{scope: child, path: "./child.modelith.yaml"}`}, "child.PaymentMethod") + "unexpected: true\n"
	files := fakeFiles{
		".git": "",
		root:   rootSource,
		child:  editedVendored(t),
	}

	results := planned(t, []Input{{Path: root, Source: []byte(rootSource)}}, files)
	if len(results) != 1 || results[0].File != root {
		t.Fatalf("results = %+v, want only the explicit root", results)
	}
	var structural bool
	for _, finding := range results[0].Findings {
		if finding.Category == CategoryStructural {
			structural = true
		}
	}
	if !structural {
		t.Errorf("root did not retain its full structural result: %+v", results[0].Findings)
	}
}

// TestADR_0017_VendoredIntermediaryReachesMismatchedVendoredGrandchild pins the
// exception to ADR-0015's import suppression: locally readable imports of a
// vendored copy participate in the integrity-only crawl, without receiving
// transitive semantic lint.
func TestADR_0017_VendoredIntermediaryReachesMismatchedVendoredGrandchild(t *testing.T) {
	t.Parallel()

	const root = "models/root.modelith.yaml"
	const middle = "models/middle.modelith.yaml"
	const child = "models/child.modelith.yaml"
	files := fakeFiles{
		".git": "",
		root:   importer([]string{`{scope: middle, path: "./middle.modelith.yaml"}`}, "middle.PaymentMethod"),
		middle: stamp(t, importer([]string{`{scope: child, path: "./child.modelith.yaml"}`}, "child.PaymentMethod")),
		child:  editedVendored(t),
	}

	results := planned(t, []Input{{Path: root, Source: []byte(files[root])}}, files)
	if len(results) != 2 || results[0].File != root || results[1].File != child || len(results[1].Findings) != 1 {
		t.Fatalf("results = %+v, want root plus the mismatched vendored grandchild", results)
	}
	if !strings.Contains(results[1].Findings[0].Message, "deps update "+child) {
		t.Errorf("grandchild = %+v, want remedy for %q", results[1], child)
	}
}

func TestPlan_ReportsReadableUnsupportedVendoredChild(t *testing.T) {
	t.Parallel()

	const root = "models/root.modelith.yaml"
	const child = "models/child.modelith.yaml"
	childSource := strings.Replace(stamp(t, gappy), "version: v1", "version: v99", 1)
	files := fakeFiles{".git": "", root: importer([]string{`"./child.modelith.yaml"`}, "child.PaymentMethod"), child: childSource}

	results := planned(t, []Input{{Path: root, Source: []byte(files[root])}}, files)
	if len(results) != 2 || results[1].File != child || len(results[1].Findings) != 1 {
		t.Errorf("results = %+v, want a provenance finding for the unsupported child", results)
	}
}

func TestPlan_PreservesExplicitInputPath(t *testing.T) {
	t.Parallel()

	const path = "./models/root.modelith.yaml"
	results := planned(t, []Input{{Path: path, Source: []byte(gappy)}}, fakeFiles{".git": ""})
	if len(results) != 1 || results[0].File != path {
		t.Errorf("results = %+v, want explicit path %q", results, path)
	}
}

func TestPlan_UsesResolvedPathForDiscoveredFinding(t *testing.T) {
	t.Parallel()

	const root = "models/root.modelith.yaml"
	const child = "models/vendor/child.modelith.yaml"
	files := fakeFiles{
		".git": "",
		root:   importer([]string{`{scope: child, path: "./vendor/../vendor/child.modelith.yaml"}`}, "child.PaymentMethod"),
		child:  editedVendored(t),
	}

	results := planned(t, []Input{{Path: root, Source: []byte(files[root])}}, files)
	if len(results) != 2 || results[1].File != child {
		t.Fatalf("results = %+v, want child attributed to resolved path %q", results, child)
	}
	if got := results[1].Findings; len(got) != 1 || !strings.Contains(got[0].Message, "deps update "+child) {
		t.Errorf("finding = %+v, want remedy for resolved path %q", got, child)
	}
}
