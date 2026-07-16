.PHONY: test build vet check-version bump release-dry release-notes release-gate smoke-matrix dogfood-help termux-meta ci

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

# Print CHANGELOG section + commits for VERSION
release-notes:
	bash scripts/release-notes.sh $(VERSION)

# W4 automated release gate
release-gate:
	bash scripts/release-gate.sh

# Batch F terminal env smoke
smoke-matrix:
	bash scripts/smoke-matrix.sh

# Emit termux-packages metadata
termux-meta:
	bash contrib/termux/package.sh

dogfood-help:
	@echo "Dogfood log: docs/dogfood/ (see docs/DOGFOOD.md)"
	@echo "Daily template: docs/dogfood/TEMPLATE.md"
	@echo "Batch B–C: docs/dogfood/BATCH_BC.md"
	@echo "Batch D–E: docs/dogfood/BATCH_DE.md"
	@echo "Batch F:   docs/dogfood/BATCH_F.md"
	@echo "Scorecard: docs/dogfood/SCORECARD.md"
	@echo "Release:   docs/RELEASE_GATE.md · make release-gate"

ci: check-version vet test build
	@echo "CI local gate OK (v$(VERSION))"
