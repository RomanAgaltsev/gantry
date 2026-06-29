// Package bluegreen deploys a two-slot environment behind a switchable pointer: Deploy
// stages the idle slot, SwitchTo flips an nginx-agnostic symlink pointer to promote it.
package bluegreen

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/executor/composessh"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

const slotEnvFile = ".env"

// Slot is one slot's compose project.
type Slot struct {
	ProjectDir   string
	ComposeFiles []string
}

// Pointer is the switchable traffic pointer: a symlink (Link) flipped between per-slot
// targets (Target[slot]), followed by Reload.
type Pointer struct {
	Link   string
	Target map[string]string
	Reload string
}

// Executor implements executor.SlotExecutor for a two-slot blue-green environment.
type Executor struct {
	Runner  composessh.Runner
	SlotMap map[string]Slot
	Order   [2]string // deterministic slot order, e.g. {"blue","green"}
	Pointer Pointer
	Logins  []composessh.RegistryLogin
}

// Slots returns the two slot names in deterministic order.
func (e *Executor) Slots() (string, string) { return e.Order[0], e.Order[1] }

// other returns the slot that is not live; an unset/unknown live slot bootstraps to the
// first slot.
func (e *Executor) other(live string) string {
	if live == e.Order[0] {
		return e.Order[1]
	}
	return e.Order[0]
}

// LiveSlot reads the pointer symlink and maps it to a slot. A missing/unreadable link means
// no live slot yet (bootstrap) and returns ("", nil).
func (e *Executor) LiveSlot(ctx context.Context) (string, error) {
	out, err := e.Runner.Run(ctx, "readlink "+composessh.ShellQuote(e.Pointer.Link), nil)
	if err != nil {
		return "", nil //nolint:nilerr // an unreadable link means "no live slot yet" (bootstrap)
	}
	target := strings.TrimSpace(out)
	if target == "" {
		return "", nil
	}
	for slot, t := range e.Pointer.Target {
		if t == target {
			return slot, nil
		}
	}
	return "", fmt.Errorf("pointer %s resolves to %q, which is not a configured slot target", e.Pointer.Link, target)
}

// Deploy stages the pin set on the idle slot's compose project.
func (e *Executor) Deploy(ctx context.Context, p executor.Plan) (executor.Result, error) {
	live, err := e.LiveSlot(ctx)
	if err != nil {
		return executor.Result{}, err
	}
	idle := e.other(live)
	slot := e.SlotMap[idle]

	envPath := path.Join(slot.ProjectDir, slotEnvFile)
	if _, err := e.Runner.Run(ctx, "cat > "+composessh.ShellQuote(envPath), pin.Render(p.Pins)); err != nil {
		return executor.Result{}, fmt.Errorf("write %s env: %w", idle, err)
	}
	if err := composessh.RunCompose(ctx, e.Runner, composessh.ComposeOpts{
		ProjectDir:   slot.ProjectDir,
		ComposeFiles: slot.ComposeFiles,
		EnvFile:      slotEnvFile,
		Logins:       e.Logins,
	}, p.Pins); err != nil {
		return executor.Result{}, err
	}
	return executor.Result{Changed: true, Detail: "blue-green deploy idle=" + idle}, nil
}
