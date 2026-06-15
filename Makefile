# https://github.com/aperturerobotics/template

SHELL:=bash
APTRE_VERSION=v0.34.0
GOFUMPT=tools/bin/gofumpt
GOIMPORTS=tools/bin/goimports
GOLANGCI_LINT=tools/bin/golangci-lint
GO_MOD_OUTDATED=tools/bin/go-mod-outdated

export GO111MODULE=on
unexport GOARCH
unexport GOOS

all:

vendor:
	go mod vendor

$(GOIMPORTS):
	cd ./tools; \
	go build -v \
		-o ./bin/goimports \
		golang.org/x/tools/cmd/goimports

$(GOFUMPT):
	cd ./tools; \
	go build -v \
		-o ./bin/gofumpt \
		mvdan.cc/gofumpt

$(GOLANGCI_LINT):
	cd ./tools; \
	go build -v \
		-o ./bin/golangci-lint \
		github.com/golangci/golangci-lint/v2/cmd/golangci-lint

$(GO_MOD_OUTDATED):
	cd ./tools; \
	go build -v \
		-o ./bin/go-mod-outdated \
		github.com/psampaz/go-mod-outdated

.PHONY: updateproto
updateproto:
	bash generator/update_protos.bash

# genproto generates the Deadlock protocol Go package from protocol/*.proto with
# the aperturerobotics/common (aptre) reflect-free protobuf-go-lite pipeline,
# Go outputs only. .tools holds the pinned generator toolchain.
.PHONY: genproto
genproto:
	[ -d .tools ] || go run -v github.com/aperturerobotics/common@$(APTRE_VERSION) .tools
	go run -mod=mod github.com/aperturerobotics/common/cmd/aptre@$(APTRE_VERSION) generate --language go

.PHONY: gendispatch
gendispatch:
	go run ./generator/dispatchgen

.PHONY: gen
gen: updateproto genproto gendispatch

.PHONY: outdated
outdated: $(GO_MOD_OUTDATED)
	go list -mod=mod -u -m -json all | $(GO_MOD_OUTDATED) -update -direct

.PHONY: lint
lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run

.PHONY: fix
fix: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run --fix

.PHONY: test
test:
	go test -v ./...

.PHONY: format
format: $(GOFUMPT) $(GOIMPORTS)
	$(GOIMPORTS) -w ./
	$(GOFUMPT) -w ./
