package s2replay

import "runtime/debug"

//go:generate go run ./scripts/parserdigest -write

// ParserSourceDigest identifies the canonical parser source and module inputs.
const ParserSourceDigest = "30ed03c72402cc1df1341bb9c0db82bc307c8c7da87260831c44a5dc4e3fdb2b"

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
