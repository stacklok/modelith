package model

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// TestADR_0011_OfflinePackages guards ADR-0011: lint, render, schema, and model
// never perform network I/O, no matter what flags are passed. It walks the
// transitive import graph rather than direct imports, because the guarantee is
// about what the code can reach, not what it names.
//
// os/exec is banned alongside the net packages on purpose: without it the rule
// could be evaded by shelling out to curl. Commands that do use the network
// live in cmd/modelith and delegate transport to git/gh.
//
// It lives here rather than in each package because the rule is one decision
// about a set of packages, and splitting it would let a new offline package be
// added without a guard. Adding one means adding it to offlinePackages.
func TestADR_0011_OfflinePackages(t *testing.T) {
	t.Parallel()

	offlinePackages := []string{
		"github.com/stacklok/modelith/internal/lint",
		"github.com/stacklok/modelith/internal/render/markdown",
		"github.com/stacklok/modelith/internal/render/mermaid",
		"github.com/stacklok/modelith/internal/schema",
		"github.com/stacklok/modelith/internal/model",
	}

	// net/url and net/netip are parsers that perform no I/O; the JSON Schema
	// library pulls them in for format validation. Everything else under net
	// dials something.
	banned := []string{"net", "net/http", "net/rpc", "net/smtp", "os/exec"}

	for _, pkg := range offlinePackages {
		// nolint:gosec // G204 flags a variable argument. pkg comes from the
		// literal offlinePackages slice above and never from input, so the
		// injection the rule guards against cannot occur.
		out, err := exec.Command("go", "list", "-deps", pkg).Output()
		if err != nil {
			t.Fatalf("go list -deps %s: %v", pkg, err)
		}
		for _, dep := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if slices.Contains(banned, dep) {
				t.Errorf("%s transitively imports %q\n"+
					"ADR-0011: lint, render, schema and model never perform network I/O.\n"+
					"See project-docs/adr/0011-network-boundary.md.", pkg, dep)
			}
		}
	}
}
