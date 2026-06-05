#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUN_SCRIPT="${ROOT_DIR}/tests/conformance/run.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

fail() {
  printf '[conformance-repo-clone-fallback-test] %s\n' "$*" >&2
  exit 1
}

assert_file_contains() {
  local path="$1"
  local pattern="$2"
  local label="$3"

  if ! grep -Fq "${pattern}" "${path}"; then
    fail "${label}: expected '${pattern}' in ${path}"
  fi
}

FAKE_BIN="${TMP_DIR}/bin"
GIT_LOG="${TMP_DIR}/git.log"
FIRST_URL="https://gh-proxy.com/https://github.com/kubernetes-sigs/gateway-api.git"
SECOND_URL="https://github.com/kubernetes-sigs/gateway-api.git"

mkdir -p "${FAKE_BIN}"
: >"${GIT_LOG}"

cat >"${FAKE_BIN}/git" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

printf 'git %s\n' "$*" >>"${FAKE_GIT_LOG}"

if [[ "$1" == "clone" ]]; then
  url="${@: -2:1}"
  dest="${@: -1}"
  if [[ "${url}" == "${FAKE_FIRST_URL}" ]]; then
    printf 'simulated 429 for %s\n' "${url}" >&2
    exit 128
  fi
  if [[ "${url}" == "${FAKE_SECOND_URL}" ]]; then
    mkdir -p "${dest}/.git"
    exit 0
  fi
fi

exit 1
EOF

chmod +x "${FAKE_BIN}/git"

CONFORMANCE_RUN_SH_SOURCE_ONLY=true source "${RUN_SCRIPT}"

PATH="${FAKE_BIN}:${PATH}"
FAKE_GIT_LOG="${GIT_LOG}"
FAKE_FIRST_URL="${FIRST_URL}"
FAKE_SECOND_URL="${SECOND_URL}"
WORK_DIR="${TMP_DIR}/gateway-api-v1.5.1"
GATEWAY_API_CLONE_URLS="${FIRST_URL},${SECOND_URL}"

export PATH FAKE_GIT_LOG FAKE_FIRST_URL FAKE_SECOND_URL WORK_DIR GATEWAY_API_CLONE_URLS

ensure_gateway_api_repo

if [[ ! -d "${WORK_DIR}/.git" ]]; then
  fail "expected fallback clone to create ${WORK_DIR}/.git"
fi

assert_file_contains "${GIT_LOG}" "${FIRST_URL}" "first clone URL attempted"
assert_file_contains "${GIT_LOG}" "${SECOND_URL}" "fallback clone URL attempted"

printf '[conformance-repo-clone-fallback-test] ok\n'
