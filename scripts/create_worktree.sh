#!/usr/bin/env bash

set -euo pipefail

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
	printf 'Usage: %s <worktree-name> <branch-name> [base-ref]\n' "$0" >&2
	exit 1
fi

worktree_name="$1"
branch_name="$2"
base_ref="${3:-origin/master}"

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

worktree_parent=".worktrees"
worktree_path="$worktree_parent/$worktree_name"

if [ ! -d "$worktree_parent" ]; then
	printf 'Missing worktree directory: %s\n' "$worktree_parent" >&2
	exit 1
fi

git fetch origin
git worktree add "$worktree_path" -b "$branch_name" "$base_ref"

bash "$repo_root/scripts/bootstrap_beads_worktree.sh" >/dev/null
bash "$worktree_path/scripts/bootstrap_beads_worktree.sh"

printf 'Worktree ready at %s\n' "$repo_root/$worktree_path"
