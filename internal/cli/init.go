package cli

import "github.com/tarrball/specval/internal/scaffold"

func cmdInit(env *Env) error {
	dir, err := env.workDir()
	if err != nil {
		return err
	}
	return scaffold.Init(dir, env.Stdout)
}
