package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

var version = "dev"
var commit = "unknown"

func versionString() string {
	revision := commit
	if revision == "unknown" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					revision = setting.Value
				}
			}
		}
	}
	return fmt.Sprintf("model-connectivity %s (commit %s, %s, %s/%s)", version, revision, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}
