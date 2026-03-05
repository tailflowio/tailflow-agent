#!/usr/bin/env bash
set -euo pipefail

# cover.sh — Run tests with coverage on productive packages only.
# Excludes: fake (mocks), pkg (types only), fx (DI wiring), metrics (Linux only), cmd (CLI).

cd "$(dirname "$0")/.."

PRODUCTIVE_PKGS=$(go list ./... | grep -v -E '(internal/fake|internal/fx|internal/metrics|pkg/|cmd/|web)')

echo "Running tests with coverage..."
go test $PRODUCTIVE_PKGS -coverprofile=coverage.out -count=1 "$@"

echo ""
echo "=== Coverage Summary ==="
go tool cover -func=coverage.out | grep total

echo ""
echo "=== Packages below 100% ==="
go tool cover -func=coverage.out | grep -v "100.0%" | grep -v "^total:" || echo "All packages at 100%!"
