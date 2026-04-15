//go:build windows

package shell

import "os/exec"

// applyProcGroupKill is a no-op on Windows.
// Process group semantics differ from Unix; the default cmd.Kill behaviour is
// sufficient for the Windows use case.
func applyProcGroupKill(_ *exec.Cmd) {}
