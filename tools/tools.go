//go:build deps_only
// +build deps_only

// Package tools pins the build-tool dependencies for s2replay.
package tools

import (
	// _ imports protowrap
	_ "github.com/aperturerobotics/goprotowrap/cmd/protowrap"
	// _ imports golangci-lint
	_ "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"
	// _ imports go-mod-outdated
	_ "github.com/psampaz/go-mod-outdated"
	// _ imports protoc-gen-go
	_ "google.golang.org/protobuf/cmd/protoc-gen-go"
	// _ imports goimports
	_ "golang.org/x/tools/cmd/goimports"
	// _ imports gofumpt
	_ "mvdan.cc/gofumpt"
)
