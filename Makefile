.PHONY: test build vet check-version bump release-dry dogfood-help

VERSION := $(shell tr -d '[:space:]' < VERSION 2>/dev/null || echo 0.0.0)

test:
	GOSUMDB=off go test ./...

vet:
	GOSUMDB=off go vet ./...

build:
	CGO_ENABLED=0 go build -ldflags="-s -w -X main.ProjectVersion=$(VERSION)" -o codeforge ./cmd/codeforge/

check-version:
	bash scripts/check-version.sh

# make bump V=1.9.0
bump:
	@test -n "$(V)" || (echo "Usage: make bump V=X.Y.Z" && exit 1)
	bash scripts/bump-version.sh $(V)

# Local goreleaser snapshot (no publish)
release-dry:
	goreleaser release --snapshot --clean --skip=publish

dogfood-help:
	@echo "Dogfood log: docs/dogfood/ (see docs/DOGFOOD.md)"
	@echo "Daily template: docs/dogfood/TEMPLATE.md"

ci: check-version vet test build
	@echo "CI local gate OK (v$(VERSION))"
