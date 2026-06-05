#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPORTS_ROOT="${REPORTS_ROOT:-${ROOT_DIR}/reports/conformance}"
REPORT_BRANCH="${1:-${REPORT_BRANCH:-conformance-reports}}"
GIT_USER_NAME="${GIT_USER_NAME:-github-actions[bot]}"
GIT_USER_EMAIL="${GIT_USER_EMAIL:-41898282+github-actions[bot]@users.noreply.github.com}"
COMMIT_MESSAGE="${COMMIT_MESSAGE:-conformance: update reports}"
DRY_RUN="${DRY_RUN:-false}"
TMP_BASE="${TMP_BASE:-${ROOT_DIR}/tmp}"

if [[ ! -d "${REPORTS_ROOT}" ]]; then
  printf 'reports directory not found: %s\n' "${REPORTS_ROOT}" >&2
  exit 1
fi

if ! git -C "${ROOT_DIR}" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  printf 'not a git work tree: %s\n' "${ROOT_DIR}" >&2
  exit 1
fi

mkdir -p "${TMP_BASE}"
WORKTREE_DIR="$(mktemp -d "${TMP_BASE}/conformance-branch.XXXXXX")"

cleanup() {
  git -C "${ROOT_DIR}" worktree remove --force "${WORKTREE_DIR}" >/dev/null 2>&1 || true
  rm -rf "${WORKTREE_DIR}"
}

trap cleanup EXIT

git -C "${ROOT_DIR}" fetch origin "${REPORT_BRANCH}:${REPORT_BRANCH}" >/dev/null 2>&1 || true

if git -C "${ROOT_DIR}" show-ref --verify --quiet "refs/heads/${REPORT_BRANCH}"; then
  git -C "${ROOT_DIR}" worktree add "${WORKTREE_DIR}" "${REPORT_BRANCH}" >/dev/null
else
  git -C "${ROOT_DIR}" worktree add --detach "${WORKTREE_DIR}" HEAD >/dev/null
  (
    cd "${WORKTREE_DIR}"
    git switch --orphan "${REPORT_BRANCH}" >/dev/null
  )
fi

(
  cd "${WORKTREE_DIR}"

  find . -mindepth 1 -maxdepth 1 ! -name .git -exec rm -rf {} +
  mkdir -p reports
  cp -R "${REPORTS_ROOT}" reports/

  cat >README.md <<EOF
# Nantian Gateway Conformance Reports

This branch is managed by automation.
The canonical source tree lives on the default development branch.

Published content:

- \`reports/conformance/latest/\`
- \`reports/conformance/releases/\`
- \`reports/conformance/runs/\`
EOF

  git config user.name "${GIT_USER_NAME}"
  git config user.email "${GIT_USER_EMAIL}"
  git add -A

  if git diff --cached --quiet; then
    printf 'no conformance report changes to publish\n'
    exit 0
  fi

  git commit -m "${COMMIT_MESSAGE}" >/dev/null
  if [[ "${DRY_RUN}" == "true" ]]; then
    printf 'dry-run: skipping push to origin/%s\n' "${REPORT_BRANCH}"
    exit 0
  fi

  git push origin "${REPORT_BRANCH}" >/dev/null
  printf 'published reports to origin/%s\n' "${REPORT_BRANCH}"
)
