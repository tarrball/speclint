package cli

import (
	"errors"
	"fmt"

	"github.com/tarrball/specval/internal/scaffold"
)

func cmdNew(env *Env, args []string) error {
	fl := newFlagSet("new", env)
	kind := fl.String("kind", "feature", "feature, bugfix, or chore")
	title := fl.String("title", "", "one-line title")
	pos, err := parseArgs(fl, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || !scaffold.ValidID(pos[0]) {
		return errors.New("usage: sv new <id>; the id starts with a letter or a digit, and contains only letters, digits, '.', '_', and '-'")
	}
	dir, err := env.workDir()
	if err != nil {
		return err
	}
	path, err := scaffold.Spec(dir, pos[0], *kind, *title)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(env.Stdout, path)
	return err
}
