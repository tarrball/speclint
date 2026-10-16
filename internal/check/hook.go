package check

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/tarrball/specval/internal/claude"
	"github.com/tarrball/specval/internal/config"
)

// HookEnv holds what HookEvent reads and writes.
type HookEnv struct {
	// Stdin holds the Claude Code PostToolUse event.
	Stdin io.Reader
	// Stdout gets the hook result that Claude Code reads as JSON.
	Stdout io.Writer
	// Stderr gets the findings when the spec does not pass.
	Stderr io.Writer
	// Dir is the working directory of specval, as an absolute path. It is ""
	// if it is not known.
	Dir string
}

// HookStatus is the result of HookEvent.
type HookStatus int

// The values of HookStatus.
const (
	// HookPass tells that Claude can continue. If the spec has warnings,
	// HookEvent wrote them to Stdout as context for Claude.
	HookPass HookStatus = iota
	// HookBlock tells that the spec has errors. HookEvent wrote them and an
	// instruction for Claude to Stderr.
	HookBlock
)

// HookEvent reads a Claude Code PostToolUse event from env.Stdin. If the event
// is about a spec, HookEvent checks the spec. If the event is not about a
// spec, it returns HookPass and writes nothing, so that the hook does not stop
// other work. It returns an error if it cannot check the spec.
func HookEvent(ctx context.Context, env HookEnv) (HookStatus, error) {
	in, err := claude.ReadEvent(env.Stdin)
	if err != nil {
		return HookPass, nil //nolint:nilerr // An event that specval cannot read is not about a spec.
	}
	fp := in.ToolInput.FilePath
	if fp == "" || (filepath.Ext(fp) != ".yaml" && filepath.Ext(fp) != ".yml") {
		return HookPass, nil
	}
	// Claude Code gives cwd in each event. If it does not, use the working
	// directory of specval. If neither is known, the path cannot be resolved.
	if in.Cwd == "" {
		in.Cwd = env.Dir
	}
	if !filepath.IsAbs(fp) {
		if !filepath.IsAbs(in.Cwd) {
			return HookPass, nil
		}
		fp = filepath.Join(in.Cwd, fp)
	}
	// A file that is not in a project is not a spec. But a configuration that
	// specval cannot read must not let a spec pass without a check.
	cfg, err := config.Find(filepath.Dir(fp))
	if errors.Is(err, config.ErrNotFound) {
		return HookPass, nil
	} else if err != nil {
		return HookPass, err
	}
	if rel, err := filepath.Rel(cfg.Abs(cfg.SpecsDir), fp); err != nil || !filepath.IsLocal(rel) {
		return HookPass, nil //nolint:nilerr // A file outside the specs directory is not a spec.
	}
	base := in.Cwd
	if base == "" {
		base = cfg.Root
	}
	// The hook reports only the findings for the spec. A problem in the
	// registry is not a problem in the spec, and Claude must not change the
	// registry to fix it. check and doctor report the registry.
	r, err := run(ctx, cfg, []string{fp}, base, false)
	if err != nil {
		return HookPass, err
	}
	if r.Errors > 0 {
		r.WriteText(env.Stderr)
		fmt.Fprintln(env.Stderr, "The spec does not pass. Apply each fix above. Then save the spec again. Do not write code until the spec passes.")
		return HookBlock, nil
	}
	if r.Warnings > 0 {
		var b strings.Builder
		r.WriteText(&b)
		if err := claude.WriteContext(env.Stdout, "The spec passes, but specval found warnings. If you can, fix them:\n"+b.String()); err != nil {
			return HookPass, err
		}
	}
	return HookPass, nil
}
