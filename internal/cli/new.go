package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/tarrball/specval/internal/baseline"
	"github.com/tarrball/specval/internal/config"
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
		return errors.New("usage: specval new <id>; the id can contain letters, digits, '.', '_', and '-'")
	}
	if kinds := baseline.Kinds(); !slices.Contains(kinds, *kind) {
		return fmt.Errorf("--kind must be one of: %s", strings.Join(kinds, ", "))
	}
	dir, err := env.workDir()
	if err != nil {
		return err
	}
	cfg, err := config.Find(dir)
	if err != nil {
		return err
	}
	path, err := scaffold.Spec(cfg, pos[0], *kind, *title)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(env.Stdout, path)
	return err
}
