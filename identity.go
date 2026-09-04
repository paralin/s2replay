package s2replay

import "runtime/debug"

//go:generate go run ./scripts/parserdigest -write

// ParserSourceDigest identifies the canonical parser source and module inputs.
const ParserSourceDigest = "849730c74679bea22fe294119f0a5e9879850858ce36dd8a3685f9f362a8a644"

// BuildRevision returns the clean VCS revision embedded in the running binary.
// It refuses unknown and modified builds because they cannot identify durable evidence.
func BuildRevision() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	var revision string
	var modified bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	return revision, revision != "" && !modified
}
