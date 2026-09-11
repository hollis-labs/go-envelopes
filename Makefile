.PHONY: build test lint vuln tidy

build:
	go build ./...

test:
	go test -race -count=1 ./...

# lint and vuln each report THREE outcomes, not two. The previous form was
#
#     @command -v tool >/dev/null 2>&1 && tool ./... || echo "tool not installed; skipping"
#
# which prints "not installed; skipping" WHEN THE TOOL RAN AND FOUND SOMETHING:
# a finding is a non-zero exit, and `||` cannot tell that from an absent binary.
# A real finding and an uninstalled tool produced the same bytes and the same
# exit 0. A gate that cannot fail is worse than no gate, because it buys false
# confidence. This was not hypothetical: the first run of the fixed target
# surfaced GO-2026-6218, which the old form had been reporting as "not
# installed; skipping" for as long as it had existed.
#
# WHAT THIS BUYS, stated precisely — `make` collapses EVERY failed recipe to
# its own exit 2, so the recipe codes below are visible only to a direct
# caller, not through `make vuln`. Measured, not assumed:
#
#   recipe exits 1 -> make exits 2
#   recipe exits 2 -> make exits 2
#
# So the guarantee is narrower than three exit codes and is the one that
# matters: EXIT 0 NOW MEANS THE TOOL RAN AND FOUND NOTHING, and nothing else.
# Previously all three outcomes exited 0. The three are told apart by message.
#
#   0  the tool ran and found nothing
#   1  the tool ran and found something
#   2  the tool did not run — examined nothing, which is NOT a pass
lint:
	go vet ./...
	@if ! command -v staticcheck >/dev/null 2>&1; then \
		echo "lint: DID NOT RUN — staticcheck absent. go install honnef.co/go/tools/cmd/staticcheck@latest"; \
		exit 2; \
	fi
	@staticcheck ./... && echo "lint: clean (staticcheck ran)" || { echo "lint: staticcheck reported findings above"; exit 1; }

vuln:
	@if ! command -v govulncheck >/dev/null 2>&1; then \
		echo "vuln: DID NOT RUN — govulncheck absent. go install golang.org/x/vuln/cmd/govulncheck@latest"; \
		exit 2; \
	fi
	@govulncheck ./... && echo "vuln: clean (govulncheck ran)" || { echo "vuln: govulncheck reported findings above"; exit 1; }

tidy:
	go mod tidy
