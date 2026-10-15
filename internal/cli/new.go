package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/tarrball/specval/internal/config"
	"github.com/tarrball/specval/internal/scaffold"
)

func cmdNew(args []string) error {
	fl := flag.NewFlagSet("new", flag.ContinueOnError)
	kind := fl.String("kind", "feature", "feature, bugfix, or chore")
	title := fl.String("title", "", "one-line title")
	pos, err := parseArgs(fl, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || !scaffold.ValidID(pos[0]) {
		return errors.New("usage: specval new <id> (letters, digits, . _ -)")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	cfg, err := config.Find(cwd)
	if err != nil {
		return err
	}
	path, err := scaffold.Spec(cfg, pos[0], *kind, *title)
	if err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}
