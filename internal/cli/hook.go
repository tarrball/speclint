package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tarrball/specval/internal/claude"
	"github.com/tarrball/specval/internal/config"
)

// cmdHook handles a Claude Code PostToolUse event. Exit 2 sends stderr back to
// Claude. Anything unrelated to specs exits 0 silently so the hook never gets
// in the way.
func cmdHook(ctx context.Context, stdin io.Reader) int {
	in, err := claude.ReadEvent(stdin)
	if err != nil {
		return 0
	}
	fp := in.ToolInput.FilePath
	if fp == "" || (filepath.Ext(fp) != ".yaml" && filepath.Ext(fp) != ".yml") {
		return 0
	}
	if !filepath.IsAbs(fp) {
		fp = filepath.Join(in.Cwd, fp)
	}
	cfg, err := config.Find(filepath.Dir(fp))
	if err != nil {
		return 0
	}
	if rel, err := filepath.Rel(cfg.Abs(cfg.SpecsDir), fp); err != nil || strings.HasPrefix(rel, "..") {
		return 0
	}
	base := in.Cwd
	if base == "" {
		base = cfg.Root
	}
	r, err := run(ctx, cfg, []string{fp}, base)
	if err != nil {
		fmt.Fprintln(os.Stderr, "specval:", err)
		return 1
	}
	if r.Errors > 0 {
		printText(os.Stderr, r, 1)
		fmt.Fprintln(os.Stderr, "The spec does not pass yet. Apply each fix above and save the spec again before writing any code.")
		return 2
	}
	if r.Warnings > 0 {
		var b strings.Builder
		printText(&b, r, 1)
		if err := claude.WriteContext(os.Stdout, "specval warnings (spec passes; consider fixing):\n"+b.String()); err != nil {
			fmt.Fprintln(os.Stderr, "specval:", err)
			return 1
		}
	}
	return 0
}
