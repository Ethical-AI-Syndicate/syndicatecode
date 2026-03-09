#!/usr/bin/env bash
set -euo pipefail

mapfile -t GO_FILES < <(git ls-files "*.go")
if [ ${#GO_FILES[@]} -eq 0 ]; then
  echo "No Go files found"
  exit 0
fi

UNFORMATTED=$(gofmt -l "${GO_FILES[@]}")
if [ -n "$UNFORMATTED" ]; then
  echo "Go files must be formatted with gofmt:"
  echo "$UNFORMATTED"
  exit 1
fi

echo "gofmt check passed"
