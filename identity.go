package s2replay

import (
	"runtime/debug"

	"golang.org/x/mod/module"
)

//go:generate go run ./scripts/parserdigest -write

// ParserSourceDigest identifies the canonical parser source and module inputs.
const ParserSourceDigest = "e6be1e8bb143f0fe3ccfefa6d7d93337dbe6f88f9bedf4eeff3795074bd404d9"

// BuildRevision identifies the parser source in the running binary. Standalone
// builds require clean VCS metadata; dependencies require a checksummed module
// pseudo-version. Local replacements and unknown revisions are refused.
func BuildRevision() (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	return parserBuildRevision(info)
}

// parserBuildRevision keeps the embedding application's revision separate from
// the parser dependency's revision. A module checksum excludes local source.
func parserBuildRevision(info *debug.BuildInfo) (string, bool) {
	const parserModule = "github.com/paralin/s2replay"
	if info.Main.Path != parserModule {
		for _, dependency := range info.Deps {
			if dependency.Path != parserModule {
				continue
			}
			if dependency.Replace != nil || dependency.Sum == "" {
				return "", false
			}
			revision, err := module.PseudoVersionRev(dependency.Version)
			return revision, err == nil && revision != ""
		}
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
