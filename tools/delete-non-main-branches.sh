#!/usr/bin/env bash
set -euo pipefail

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "Error: this script must be run inside a Git repository." >&2
  exit 1
fi

if ! git show-ref --verify --quiet refs/heads/main; then
  echo "Error: local branch 'main' was not found." >&2
  exit 1
fi

current_branch="$(git branch --show-current || true)"
if [[ "$current_branch" != "main" ]]; then
  echo "Switching to 'main' from '$current_branch'..."
  git switch main >/dev/null 2>&1 || git checkout main >/dev/null 2>&1
fi

mapfile -t branches_to_delete < <(git branch --format='%(refname:short)' | sed '/^main$/d')

if [[ ${#branches_to_delete[@]} -eq 0 ]]; then
  echo "No local branches to delete."
  exit 0
fi

echo "Deleting local branches (except 'main'):"
printf ' - %s\n' "${branches_to_delete[@]}"

for branch in "${branches_to_delete[@]}"; do
  git branch -D "$branch"
done

echo "Done."
