# Contributing

## Development

s2replay uses Go and the aperturerobotics template build flow.

Build-tool binaries are pinned in `tools/` and built on demand into
`tools/bin/`. Common targets:

- `make gen` regenerates protobuf bindings from the vendored Deadlock protos.
- `make format` runs goimports and gofumpt.
- `make lint` runs golangci-lint.
- `make test` runs the Go test suite.

## Commits

Sign off every commit with `git commit -s` and use conventional commit
messages: `type(scope): description` where type is one of `feat`, `fix`,
`refactor`, `chore`, `docs`, `test`, or `perf`.
