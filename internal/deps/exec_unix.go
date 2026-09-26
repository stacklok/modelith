//go:build !windows

package deps

import (
	"context"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// Run executes name with args and returns its standard output. Standard error
// is folded into the returned error, because the CLI reports why it refused there.
//
// The child runs in its own process group and is killed as a group on context
// cancellation. CommandContext alone kills only the direct child, and a CLI
// like az that spawns a helper (token refresh, keychain) can leave that helper
// holding the stdout/stderr pipe — Wait then blocks forever past the deadline,
// which is the macOS stall this guards against. WaitDelay bounds that last
// wait regardless, so a helper that escaped the group still cannot hold Wait
// hostage.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	// nolint:gosec // G204 flags a variable command and arguments, which is
	// what a transport seam is. The commands are the literals "gh" and "az" at
	// the call sites; the arguments are literals plus endpoints assembled from
	// URLs that ParseSource has already validated. Nothing here comes from a
	// model file, and there is no shell: exec passes an argv array.
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// The negative pid names the process group, so a helper the CLI
		// spawned dies with it instead of keeping the pipe open past the
		// deadline. ESRCH is fine — the process may already be gone.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second
	var stderr strings.Builder
	cmd.Stderr = &stderr
	return runCommand(cmd, name, args, &stderr)
}
