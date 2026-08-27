package s2replay

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestParserSourceDigestIsCurrent(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, name := range []string{"go.mod", "go.sum"} {
		names = append(names, name)
	}
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			if info.Name() == ".git" || info.Name() == "tools" || info.Name() == ".tmp" {
				return filepath.SkipDir
			}
			return nil
		}
		if (filepath.Ext(path) != ".go" && filepath.Ext(path) != ".proto") || (filepath.Ext(path) == ".go" && strings.HasSuffix(path, "_test.go")) || filepath.Base(path) == "identity.go" || (filepath.Ext(path) == ".proto" && !strings.HasPrefix(filepath.ToSlash(path), "protocol/")) {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != ParserSourceDigest {
		t.Fatalf("parser source digest stale: got %s want %s", got, ParserSourceDigest)
	}
}
