package main

import (
	"runtime"
	"runtime/debug"
)

// versionInfo contains version and build information.
type versionInfo struct {
	Version    string
	CommitTime string
	GoVersion  string
	Platform   string
}

// getVersionInfo returns the current version information.
func getVersionInfo() versionInfo {
	buildVersion := "unknown"
	commitTime := "unknown"
	goVer := runtime.Version()

	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "(devel)" && info.Main.Version != "" {
			buildVersion = info.Main.Version
		}

		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.time":
				commitTime = setting.Value
			}
		}
	}

	return versionInfo{
		Version:    buildVersion,
		CommitTime: commitTime,
		GoVersion:  goVer,
		Platform:   runtime.GOOS + "/" + runtime.GOARCH,
	}
}
