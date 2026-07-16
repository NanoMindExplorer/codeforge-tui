.PHONY: test build vet fmt fmt-check install-hooks check-version bump release-dry release-notes release-gate smoke-matrix dogfood dogfood-help termux-meta ci

VERSION := $(shell tr -d '[:space:]' < VERSION 2>/dev/null || echo 0.0.0)

test:
	GOSUMDB=off go test ./...

vet:
	GOSUMDB=off go vet ./...

# Format all Go sources (run before commit if hooks not installed)
fmt:
	gofmt -w .

# Fail if any file needs gofmt (CI / release-gate)
fmt-check:
	bash scripts/gofmt-check.sh

# Enable repo pre-commit hook (gofmt staged *.go)
install-hooks:
	bash scripts/install-hooks.sh

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

# Field + automated dogfood evidence → docs/dogfood/RESULTS.md
dogfood:
	bash scripts/dogfood-run.sh

dogfood-help:
	@echo "Dogfood program: docs/dogfood/PROGRAM.md (10 working days)"
	@echo "Run evidence:    make dogfood   (DOGFOOD_LIVE=0 to skip live API)"
	@echo "Results:         docs/dogfood/RESULTS.md"
	@echo "Daily template:  docs/dogfood/TEMPLATE.md"
	@echo "Checklist:       docs/DOGFOOD.md"
	@echo "Scorecard:       docs/dogfood/SCORECARD.md"
	@echo "Batches:         BATCH_BC / BATCH_DE / BATCH_F"
	@echo "Release gate:    docs/RELEASE_GATE.md · make release-gate"

ci: check-version fmt-check vet test build
	@echo "CI local gate OK (v$(VERSION))"
