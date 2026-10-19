package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tarrball/speclint/internal/baseline"
	"github.com/tarrball/speclint/internal/scaffold"
)

func cmdNew(_ context.Context, env *Env, args []string) error {
	fl := newFlagSet("new", env)
	kinds := baseline.Kinds()
	kind := fl.String("kind", kinds[0], "the kind of the change: `"+strings.Join(kinds, "|")+"`")
	title := fl.String("title", "", "the title of the spec, on one line: `text`")
	pos, err := parseArgs(fl, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || !scaffold.ValidID(pos[0]) {
		return errors.New("usage: slint new <id>; " + scaffold.IDRule)
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
