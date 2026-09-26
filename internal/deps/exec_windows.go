//go:build windows

package deps

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// Run executes name with args and returns its standard output. Standard error
// is folded into the returned error, because the CLI reports why it refused there.
//
// Windows has no POSIX process groups, so the group kill that !windows builds
// use has no equivalent here: CommandContext still terminates the direct child
// on cancellation, and WaitDelay bounds the wait even if a helper the CLI
// spawned keeps the pipe open.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	// nolint:gosec // G204 flags a variable command and arguments, which is
	// what a transport seam is. The commands are the literals "gh" and "az" at
	// the call sites; the arguments are literals plus endpoints assembled from
	// URLs that ParseSource has already validated. Nothing here comes from a
	// model file, and there is no shell: exec passes an argv array.
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 5 * time.Second
	var stderr strings.Builder
	cmd.Stderr = &stderr
	return runCommand(cmd, name, args, &stderr)
}
