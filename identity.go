package s2replay

import "runtime/debug"

//go:generate go run ./scripts/parserdigest -write

// ParserSourceDigest identifies the canonical parser source and module inputs.
const ParserSourceDigest = "e181462dd7ce6b17abf5ff47c079ad00a9ad580b3847b1b722d363aee02e8e58"

// BuildRevision returns the clean VCS revision embedded in the running binary.
// It refuses unknown and modified builds because they cannot identify durable evidence.
func BuildRevision() (string, bool) {
	// Read the build metadata supplied by the Go linker.
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}

	// Accept only a recorded revision from an unmodified source tree.
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
