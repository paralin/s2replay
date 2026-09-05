// Command parserdigest computes the canonical parser source digest.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	// Parse flags and resolve the repository root to hash.
	write := flag.Bool("write", false, "rewrite identity.go with the computed digest")
	check := flag.Bool("check", false, "fail when identity.go or source state is stale/dirty")
	flag.Parse()
	root, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	// Collect the name of every file that contributes to parser behavior.
	names := []string{"go.mod", "go.sum"}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Skip directories that hold no parser behavior.
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == ".tmp" || entry.Name() == "tools" {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip files outside the digest: non-source files, tests, the
		// digest declaration itself, and protos outside the protocol
		// directory.
		if (filepath.Ext(path) != ".go" && filepath.Ext(path) != ".proto") ||
			(filepath.Ext(path) == ".go" && strings.HasSuffix(path, "_test.go")) ||
			filepath.Base(path) == "identity.go" ||
			(filepath.Ext(path) == ".proto" && !strings.HasPrefix(filepath.ToSlash(path), "protocol/")) {
			return nil
		}

		// Record the file by its repo-relative slash path.
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		panic(err)
	}

	// Hash each file name and content in sorted order.
	sort.Strings(names)
	h := sha256.New()
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			panic(err)
		}
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}
	digest := hex.EncodeToString(h.Sum(nil))
	fmt.Println(digest)

	// In check mode, fail when identity.go holds a stale digest or the
	// behavior source carries uncommitted or untracked changes.
	if *check {
		data, err := os.ReadFile(filepath.Join(root, "identity.go"))
		if err != nil {
			panic(err)
		}
		if !strings.Contains(string(data), `ParserSourceDigest = "`+digest+`"`) {
			panic("stale parser source digest")
		}
		if output, err := exec.Command("git", "diff", "--quiet", "--", ".", ":(exclude)identity.go").CombinedOutput(); err != nil {
			panic("behavior source is dirty: " + string(output))
		}
		if output, err := exec.Command("git", "status", "--porcelain", "--untracked-files=all").Output(); err == nil && strings.TrimSpace(string(output)) != "" {
			panic("behavior source is dirty: " + string(output))
		}
		return
	}
	if !*write {
		return
	}

	// Rewrite the digest value inside identity.go, preserving the rest of
	// the file text.
	path := filepath.Join(root, "identity.go")
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	text := string(data)
	start := strings.Index(text, "ParserSourceDigest = \"")
	if start < 0 {
		panic("ParserSourceDigest declaration missing")
	}
	start += len("ParserSourceDigest = \"")
	end := strings.Index(text[start:], "\"")
	if end < 0 {
		panic("ParserSourceDigest value missing")
	}
	end += start
	text = text[:start] + digest + text[end:]
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		panic(err)
	}
}
