package s2replay

import "runtime/debug"

//go:generate go run ./scripts/parserdigest -write

// ParserSourceDigest identifies the canonical parser source and module inputs.
const ParserSourceDigest = "50ba8ac21dd36fc461f862b9bfbf05ff4f5ad0531e6375c0db215bf622035133"

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
