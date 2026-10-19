// Command speclint validates change specs against architecture rules before
// a developer writes code.
package main

import (
	"context"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/tarrball/speclint/internal/cli"
)

// version is the version of a release binary. The release build sets it
// with -ldflags "-X main.version=v0.1.0". go install leaves it empty, and
// the build information gives the version.
var version string

func main() {
	// On an interrupt or a terminate signal, ctx stops the evaluation of the
	// rules and the read of the hook event, and slint exits. After the first
	// signal, stop restores the default handling, so that a second signal
	// ends the process if the work does not stop.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop()
	}()
	dir, _ := os.Getwd()        // If the directory is not known, Dir is "", and only the commands that need it fail.
	home, _ := os.UserHomeDir() // If there is no home directory, Home is "".
	exe, _ := os.Executable()   // If the path is not known, Executable is "".
	info, _ := debug.ReadBuildInfo()
	code := cli.Run(ctx, os.Args[1:], &cli.Env{
		Stdin:      os.Stdin,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
		Dir:        dir,
		Home:       home,
		Executable: exe,
		Version:    versionOf(version, info),
	})
	stop()
	os.Exit(code)
}

// versionOf returns the version of the binary: the version that the release
// build set, the module version from go install, or the VCS revision of a
// local build. It returns "" if none is known.
func versionOf(release string, info *debug.BuildInfo) string {
	if release != "" {
		return release
	}
	if info == nil {
		return ""
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var rev, modified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	if rev == "" {
		return ""
	}
	rev = rev[:min(12, len(rev))]
	if modified == "true" {
		rev += "-dirty"
	}
	return "devel-" + rev
}
