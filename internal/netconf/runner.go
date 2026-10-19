package netconf

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Runner executes an external system command and returns its stdout. It
// exists so --dry-run can swap in a recorder without touching call sites.
type Runner interface {
	Run(name string, args ...string) (string, error)
}

// ExecRunner runs commands for real.
type ExecRunner struct{}

// Run executes name with args and returns trimmed stdout; a non-zero exit
// yields a command_failed error that embeds stderr.
func (ExecRunner) Run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", &Error{Code: CodeCommandFailed,
			Err: fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), msg)}
	}
	return strings.TrimSpace(stdout.String()), nil
}

// DryRunner records commands instead of executing them; every call reports
// success so flows proceed to later planning steps.
type DryRunner struct {
	Commands []string
}

// Run appends the command to the log and returns an empty stdout.
func (d *DryRunner) Run(name string, args ...string) (string, error) {
	d.Commands = append(d.Commands, name+" "+strings.Join(args, " "))
	return "", nil
}

// lastErr is a helper for platform code: wrap "not found" outcomes.
func ifaceNotFound(name string) error {
	return newError(CodeNotFound, "interface %q not found", name)
}

// errNil adapts (string, error) returns to plain error returns.
func errNil(_ string, err error) error { return err }

// itoa keeps platform files free of a strconv import for one call.
func itoa(n int) string { return strconv.Itoa(n) }
