#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"

# shellcheck source=scripts/lib/common.sh
source "${repo_root}/scripts/lib/common.sh"

usage() {
  cat <<'EOF'
usage: check-community-governance-contract.sh [--repo-root <path>]

Verifies that the public community governance contract is linked from the
repository entry points and still covers contribution, conduct, security,
support, release, compatibility, roadmap, and adopter evidence.
EOF
}

log() {
  printf '[community-governance] %s\n' "$*"
}

require_pattern() {
  local file="$1"
  local pattern="$2"
  local label="$3"

  aeg_require_file "${file}"
  if ! grep -Eq -- "${pattern}" "${file}"; then
    aeg_fail "${file} is missing ${label}: ${pattern}"
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      [[ $# -ge 2 ]] || {
        usage >&2
        aeg_usage_error "missing value for --repo-root"
      }
      repo_root="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      aeg_usage_error "unknown argument: $1"
      ;;
  esac
done

readme_path="${repo_root}/README.md"
community_path="${repo_root}/docs/community-readiness.md"
adopters_path="${repo_root}/docs/adopters-and-compatibility.md"

for file in \
  "${readme_path}" \
  "${repo_root}/CONTRIBUTING.md" \
  "${repo_root}/CODE_OF_CONDUCT.md" \
  "${repo_root}/SECURITY.md" \
  "${repo_root}/SUPPORT.md" \
  "${repo_root}/VERSIONING.md" \
  "${repo_root}/ROADMAP.md" \
  "${repo_root}/GOVERNANCE.md" \
  "${repo_root}/MAINTAINERS.md" \
  "${community_path}" \
  "${adopters_path}"; do
  aeg_require_file "${file}"
done

for section in \
  'What Is Supported' \
  'What Is Not Supported Yet' \
  'Install' \
  'Verify' \
  'Troubleshooting' \
  'Uninstall'; do
  require_pattern "${readme_path}" "^## ${section}$" "README section ${section}"
done

for link in \
  'CONTRIBUTING.md' \
  'CODE_OF_CONDUCT.md' \
  'SECURITY.md' \
  'SUPPORT.md' \
  'VERSIONING.md' \
  'ROADMAP.md' \
  'GOVERNANCE.md' \
  'MAINTAINERS.md' \
  'docs/adopters-and-compatibility.md' \
  'docs/community-readiness.md' \
  'docs/gateway-api-support.md' \
  'docs/user/getting-started.md' \
  'docs/user/operations.md'; do
  require_pattern "${readme_path}" "${link}" "README link ${link}"
done

for policy in \
  'Contribution Guide' \
  'Code of Conduct' \
  'Support Policy' \
  'Security Policy' \
  'Versioning &amp; Release Policy' \
  'Compatibility Contract' \
  'Roadmap' \
  'Public adopter / case study / compatibility matrix page'; do
  require_pattern "${community_path}" "${policy}" "community readiness policy ${policy}"
done

require_pattern "${adopters_path}" 'named adopter' "adopter evidence boundary"
require_pattern "${adopters_path}" 'public case study' "case study evidence boundary"
require_pattern "${adopters_path}" 'public compatibility matrix' "compatibility matrix"
require_pattern "${repo_root}/VERSIONING.md" 'Release Triggers' "release policy trigger section"
require_pattern "${repo_root}/VERSIONING.md" 'Compatibility Expectations' "compatibility expectation section"
require_pattern "${repo_root}/SUPPORT.md" 'Which Issue Form To Use' "support policy issue routing"
require_pattern "${repo_root}/SECURITY.md" 'Reporting' "security reporting policy"
require_pattern "${repo_root}/ROADMAP.md" '^## v0\.2' "v0.2 roadmap section"
require_pattern "${repo_root}/ROADMAP.md" '^## v0\.3' "v0.3 roadmap section"
require_pattern "${repo_root}/ROADMAP.md" '^## v0\.4' "v0.4 roadmap section"

log "community governance contract aligned"
