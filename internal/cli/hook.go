package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tarrball/specval/internal/claude"
	"github.com/tarrball/specval/internal/config"
)

// Exit codes of specval hook. Claude Code gives them these meanings.
const (
	hookPass  = 0 // Claude continues. Claude Code reads stdout as JSON for Claude.
	hookError = 1 // Claude Code shows stderr to the user, and Claude continues.
	hookBlock = 2 // Claude Code sends stderr to Claude.
)

// cmdHook handles a Claude Code PostToolUse event. If the event is not about
// a spec, it exits 0 and writes nothing, so that the hook does not get in the
// way of other work.
func cmdHook(ctx context.Context, env *Env) int {
	in, err := claude.ReadEvent(env.Stdin)
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
	cfg, err := config.Find(filepath.Dir(fp))
	if err != nil {
		return hookPass
	}
	if rel, err := filepath.Rel(cfg.Abs(cfg.SpecsDir), fp); err != nil || !filepath.IsLocal(rel) {
		return hookPass
	}
	base := in.Cwd
	if base == "" {
		base = cfg.Root
	}
	r, err := run(ctx, cfg, []string{fp}, base)
	if err != nil {
		fmt.Fprintln(env.Stderr, "specval:", err)
		return hookError
	}
	if r.Errors > 0 {
		printText(env.Stderr, r, 1)
		fmt.Fprintln(env.Stderr, "The spec does not pass yet. Apply each fix above and save the spec again before writing any code.")
		return hookBlock
	}
	if r.Warnings > 0 {
		var b strings.Builder
		printText(&b, r, 1)
		if err := claude.WriteContext(env.Stdout, "specval warnings (spec passes; consider fixing):\n"+b.String()); err != nil {
			fmt.Fprintln(env.Stderr, "specval:", err)
			return hookError
		}
	}
	return hookPass
}
