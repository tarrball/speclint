package main

import (
	"runtime/debug"
	"testing"
)

func TestVersionOf(t *testing.T) {
	settings := func(revision, modified string) []debug.BuildSetting {
		return []debug.BuildSetting{{Key: "vcs.revision", Value: revision}, {Key: "vcs.modified", Value: modified}}
	}
	devel := debug.Module{Version: "(devel)"}
	for _, c := range []struct {
		name    string
		release string
		info    *debug.BuildInfo
		want    string
	}{
		{"release build", "v0.1.0", &debug.BuildInfo{Main: debug.Module{Version: "v0.0.9"}}, "v0.1.0"},
		{"go install", "", &debug.BuildInfo{Main: debug.Module{Version: "v0.1.0"}}, "v0.1.0"},
		{"local build", "", &debug.BuildInfo{Main: devel, Settings: settings("0123456789abcdef", "false")}, "devel-0123456789ab"},
		{"local build with changes", "", &debug.BuildInfo{Main: devel, Settings: settings("0123456789abcdef", "true")}, "devel-0123456789ab-dirty"},
		{"no VCS", "", &debug.BuildInfo{Main: devel}, ""},
		{"no build information", "", nil, ""},
	} {
		if got := versionOf(c.release, c.info); got != c.want {
			t.Errorf("%s: versionOf = %q, want %q", c.name, got, c.want)
		}
	}
}
