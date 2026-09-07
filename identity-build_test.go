package s2replay

import (
	"runtime/debug"
	"testing"
)

// TestParserBuildRevisionDoesNotUseEmbeddingApplication verifies that saved
// evidence identifies the parser dependency even when the application is dirty.
func TestParserBuildRevisionDoesNotUseEmbeddingApplication(t *testing.T) {
	const parserModule = "github.com/paralin/s2replay"
	const parserRevision = "11c0f5d4e3ef"
	info := debug.BuildInfo{
		Main: debug.Module{Path: "example.com/application"},
		Deps: []*debug.Module{{
			Path: parserModule, Version: "v0.0.0-20260906120000-" + parserRevision,
			Sum: "h1:immutable-module-checksum",
		}},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "application-revision"},
			{Key: "vcs.modified", Value: "true"},
		},
	}
	if revision, ok := parserBuildRevision(&info); !ok || revision != parserRevision {
		t.Fatalf("parser dependency revision = %q, %v", revision, ok)
	}

	// A local replacement has no immutable source identity, even when the
	// replaced requirement retains its original version and checksum.
	info.Deps[0].Replace = &debug.Module{Path: "../parser"}
	if revision, ok := parserBuildRevision(&info); ok || revision != "" {
		t.Fatalf("accepted local replacement: %q, %v", revision, ok)
	}
	info.Deps[0].Replace = nil
	info.Deps[0].Sum = ""
	if revision, ok := parserBuildRevision(&info); ok || revision != "" {
		t.Fatalf("accepted unchecked dependency: %q, %v", revision, ok)
	}
	info.Deps = nil
	if revision, ok := parserBuildRevision(&info); ok || revision != "" {
		t.Fatalf("used unrelated application revision: %q, %v", revision, ok)
	}
}

// TestParserBuildRevisionRequiresCleanStandaloneSource preserves the standalone
// extractor's refusal of modified and unidentified source builds.
func TestParserBuildRevisionRequiresCleanStandaloneSource(t *testing.T) {
	info := debug.BuildInfo{
		Main: debug.Module{Path: "github.com/paralin/s2replay"},
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "parser-revision"},
			{Key: "vcs.modified", Value: "false"},
		},
	}
	if revision, ok := parserBuildRevision(&info); !ok || revision != "parser-revision" {
		t.Fatalf("clean standalone revision = %q, %v", revision, ok)
	}
	info.Settings[1].Value = "true"
	if _, ok := parserBuildRevision(&info); ok {
		t.Fatal("accepted modified standalone parser")
	}
	info.Settings = nil
	if revision, ok := parserBuildRevision(&info); ok || revision != "" {
		t.Fatalf("accepted unidentified standalone parser: %q, %v", revision, ok)
	}
}
