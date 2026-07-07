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
	"github.com/RomanAgaltsev/gantry/internal/verify"
)

var (
	_ executor.SlotExecutor    = (*Executor)(nil)
	_ verify.ComposeVerifiable = (*Executor)(nil)
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

// noLinkSentinel is echoed by the pointer probe when the link does not exist, letting
// LiveSlot tell a genuine bootstrap (no pointer yet) apart from a transport failure. It
// must be a string readlink can never produce for a configured target.
const noLinkSentinel = "__gantry_no_link__"

// LiveSlot reads the pointer symlink and maps it to a slot. A genuinely absent pointer means
// no live slot yet (bootstrap) and returns ("", nil). A runner/transport error is propagated
// rather than masked as bootstrap, so a flaky SSH connection can't silently make Deploy stage
// the wrong slot.
func (e *Executor) LiveSlot(ctx context.Context) (string, error) {
	link := composessh.ShellQuote(e.Pointer.Link)
	probe := fmt.Sprintf("if [ -L %s ]; then readlink %s; else echo %s; fi", link, link, noLinkSentinel)
	out, err := e.Runner.Run(ctx, probe, nil)
	if err != nil {
		return "", fmt.Errorf("read pointer %s: %w", e.Pointer.Link, err)
	}
	target := strings.TrimSpace(out)
	if target == "" || target == noLinkSentinel {
		return "", nil // no pointer yet: bootstrap
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

// SwitchTo atomically flips the pointer symlink to the slot's target, then reloads.
func (e *Executor) SwitchTo(ctx context.Context, slot string) error {
	target, ok := e.Pointer.Target[slot]
	if !ok {
		return fmt.Errorf("unknown slot %q", slot)
	}
	tmp := e.Pointer.Link + ".tmp"
	flip := fmt.Sprintf("ln -sfn %s %s && mv -Tf %s %s",
		composessh.ShellQuote(target), composessh.ShellQuote(tmp),
		composessh.ShellQuote(tmp), composessh.ShellQuote(e.Pointer.Link))
	if _, err := e.Runner.Run(ctx, flip, nil); err != nil {
		return fmt.Errorf("flip pointer to %s: %w", slot, err)
	}
	if _, err := e.Runner.Run(ctx, e.Pointer.Reload, nil); err != nil {
		return fmt.Errorf("reload after switch to %s: %w", slot, err)
	}
	return nil
}

// ComposeTarget resolves the idle slot's compose project (the slot Deploy stages), so a
// compose-ps probe checks the slot about to be promoted rather than the live one.
func (e *Executor) ComposeTarget(ctx context.Context) (verify.ComposeTarget, error) {
	live, err := e.LiveSlot(ctx)
	if err != nil {
		return verify.ComposeTarget{}, err
	}
	idle := e.other(live)
	s := e.SlotMap[idle]
	return verify.ComposeTarget{ProjectDir: s.ProjectDir, ComposeFiles: s.ComposeFiles, EnvFile: slotEnvFile}, nil
}

// CloseRunner releases the executor's runner connection if the runner supports it.
func (e *Executor) CloseRunner() error {
	if c, ok := e.Runner.(composessh.Closer); ok {
		return c.Close()
	}
	return nil
}
