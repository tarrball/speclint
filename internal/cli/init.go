package cli

import "github.com/tarrball/specval/internal/scaffold"

func cmdInit(env *Env) error {
	return scaffold.Init(env.Dir, env.Stdout)
}
