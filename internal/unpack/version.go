package unpack

import "runtime/debug"

const version = "0.3.0"

var buildTime = "unknown"

// vcsInfo reads the git revision, commit time, and dirty flag that `go build`
// embeds automatically (via -buildvcs) instead of injecting them via -ldflags.
func vcsInfo() (revision, commitTime string, modified bool) {
	var (
		info *debug.BuildInfo
		ok   bool
	)

	revision = "none"
	commitTime = "unknown"

	info, ok = debug.ReadBuildInfo()
	if !ok {
		return revision, commitTime, modified
	}

	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.time":
			commitTime = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}

	return revision, commitTime, modified
}
