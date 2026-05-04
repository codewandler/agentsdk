#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

echo "==> go test ./..."
go test ./...

echo "==> nested example module tests"
(
  cd examples/devops-cli
  go test ./...
)
(
  cd examples/research-desk
  go test ./...
)

echo "==> guard: removed standard bundles stay deleted"
test ! -d tools/standard
test ! -d plugins/standard
if git ls-files 'tools/standard/**' 'plugins/standard/**' | grep -q .; then
  echo "removed standard bundle files are still tracked" >&2
  exit 1
fi
if grep -R --include='*.go' -nE 'tools/standard|plugins/standard' .; then
  echo "Go code must not import removed standard bundles" >&2
  exit 1
fi

echo "==> guard: terminal command results are not obviously ignored"
if grep -R --include='*.go' -nE '(_|_,[[:space:]]*err)[[:space:]]*:?=[[:space:]]*.*(ExecuteCommand|\.Execute\(ctx, input\)|reg\.Execute)' terminal cmd; then
  echo "terminal/cmd command execution result appears ignored; render or handle command.Result explicitly" >&2
  exit 1
fi

echo "release readiness checks passed"
