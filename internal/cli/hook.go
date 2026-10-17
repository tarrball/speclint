package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tarrball/specval/internal/check"
	"github.com/tarrball/specval/internal/claude"
)

// Exit codes of sv hook. Claude Code gives them these meanings.
const (
	hookPass  exitCode = 0 // Claude continues. Claude Code reads stdout as JSON for Claude.
	hookError exitCode = 1 // Claude Code shows stderr to the user, and Claude continues.
	hookBlock exitCode = 2 // Claude Code sends stderr to Claude.
)

// readEvent reads the hook event from stdin. It returns when ctx is done,
// also if stdin stays open and the event is not complete. Then the read
// continues in the background until the process exits.
func readEvent(ctx context.Context, env *Env) (claude.Event, error) {
	type result struct {
		in  claude.Event
		err error
	}
	ch := make(chan result, 1)
	go func() {
		in, err := claude.ReadEvent(env.Stdin)
		ch <- result{in, err}
	}()
	select {
	case r := <-ch:
		return r.in, r.err
	case <-ctx.Done():
		return claude.Event{}, ctx.Err()
	}
}

// cmdHook handles a Claude Code PostToolUse event. If the event is not about
// a spec, it exits 0 and writes nothing, so that the hook does not stop other
// work.
func cmdHook(ctx context.Context, env *Env) exitCode {
	in, err := readEvent(ctx, env)
	if ctx.Err() != nil {
		fmt.Fprintln(env.Stderr, "sv: stopped before the hook event was read:", ctx.Err())
		return hookError
	}
	if err != nil {
		return hookPass
	}
	fp := in.ToolInput.FilePath
	if fp == "" || (filepath.Ext(fp) != ".yaml" && filepath.Ext(fp) != ".yml") {
		return hookPass
	}
	// Claude Code gives cwd in each event. If it does not, use the working
	// directory of specval. If neither is known, the path cannot be resolved.
	if in.Cwd == "" {
		in.Cwd = env.Dir
	}
	if !filepath.IsAbs(fp) {
		if !filepath.IsAbs(in.Cwd) {
			return hookPass
		}
		fp = filepath.Join(in.Cwd, fp)
	}
	// The hook reports only the findings for the spec. A problem in the
	// registry is not a problem in the spec, and Claude must not change the
	// registry to fix it. check and doctor report the registry.
	r, err := check.SpecFile(ctx, fp, in.Cwd)
	if err != nil {
		fmt.Fprintln(env.Stderr, "sv:", err)
		return hookError
	}
	if r == nil {
		return hookPass
	}
	if r.Errors > 0 {
		printText(env.Stderr, r)
		fmt.Fprintln(env.Stderr, "The spec does not pass. Apply each fix above. Then save the spec again. Do not write code until the spec passes.")
		return hookBlock
	}
	if r.Warnings > 0 {
		var b strings.Builder
		printText(&b, r)
		if err := claude.WriteContext(env.Stdout, "The spec passes, but specval found warnings. If you can, fix them:\n"+b.String()); err != nil {
			fmt.Fprintln(env.Stderr, "sv:", err)
			return hookError
		}
	}
	return hookPass
}
