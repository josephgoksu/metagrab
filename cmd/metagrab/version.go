package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// Set by the release build with -ldflags "-X main.version=... -X main.commit=...".
var (
	version = ""
	commit  = ""
)

func versionString() string {
	v, c := version, commit
	if info, ok := debug.ReadBuildInfo(); ok {
		if v == "" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			v = info.Main.Version // set by `go install ...@vX.Y.Z`
		}
		if c == "" {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" && len(s.Value) >= 7 {
					c = s.Value[:7]
				}
			}
		}
	}
	if v == "" {
		v = "dev"
	}
	if c != "" {
		return fmt.Sprintf("metagrab %s (%s, %s)", v, c, runtime.Version())
	}
	return fmt.Sprintf("metagrab %s (%s)", v, runtime.Version())
}
