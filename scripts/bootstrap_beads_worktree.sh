#!/usr/bin/env bash

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

if [ ! -d ".beads" ]; then
	printf 'No .beads directory found at %s\n' "$repo_root" >&2
	exit 1
fi

if bd ready --json >/dev/null 2>&1; then
	printf 'Beads is already healthy in %s\n' "$repo_root"
	exit 0
fi

metadata_file=".beads/metadata.json"
db_name=""
if [ -f "$metadata_file" ]; then
	db_name="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("dolt_database",""))' "$metadata_file" 2>/dev/null || true)"
fi

if [ -z "$db_name" ]; then
	db_name="$(basename "$repo_root")"
fi

if [ ! -d ".beads/dolt/$db_name" ]; then
	donor_path=""
	while IFS= read -r line; do
		case "$line" in
			worktree\ *)
				candidate="${line#worktree }"
				if [ "$candidate" = "$repo_root" ]; then
					continue
				fi
				if [ -d "$candidate/.beads/dolt/$db_name" ]; then
					donor_path="$candidate"
					break
				fi
				;;
		esac
	done < <(git worktree list --porcelain)

	if [ -z "$donor_path" ]; then
		printf 'Unable to locate donor worktree with .beads/dolt/%s\n' "$db_name" >&2
		exit 1
	fi

	mkdir -p ".beads/dolt"
	cp -a "$donor_path/.beads/dolt/$db_name" ".beads/dolt/"
	printf 'Copied beads database from %s\n' "$donor_path"
fi

bd dolt stop >/dev/null 2>&1 || true
bd dolt start >/dev/null

bd ready --json >/dev/null
printf 'Beads bootstrap complete for %s\n' "$repo_root"
