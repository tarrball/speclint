package cli

import (
	"context"
	"fmt"

	"github.com/tarrball/specval/internal/check"
)

// Exit codes of sv hook. Claude Code gives them these meanings.
const (
	hookPass  exitCode = 0 // Claude continues. Claude Code reads stdout as JSON for Claude.
	hookError exitCode = 1 // Claude Code shows stderr to the user, and Claude continues.
	hookBlock exitCode = 2 // Claude Code sends stderr to Claude.
)

// cmdHook handles a Claude Code PostToolUse event. If the event is not about
// a spec, it exits 0 and writes nothing, so that the hook does not stop other
// work.
func cmdHook(ctx context.Context, env *Env) exitCode {
	status, err := check.HookEvent(ctx, check.HookEnv{
		Stdin:  env.Stdin,
		Stdout: env.Stdout,
		Stderr: env.Stderr,
		Dir:    env.Dir,
	})
	if err != nil {
		fmt.Fprintln(env.Stderr, "sv:", err)
		return hookError
	}
	if status == check.HookBlock {
		return hookBlock
	}
	return hookPass
}
