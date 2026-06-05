#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
RUN_SCRIPT="${ROOT_DIR}/tests/conformance/run.sh"

fail() {
  printf '[conformance-listener-relay-defaults-test] %s\n' "$*" >&2
  exit 1
}

if ! grep -Fq 'ADDITIONAL_TCP_LISTENER_PORTS="${ADDITIONAL_TCP_LISTENER_PORTS:-8080,8090,8443,8883}"' "${RUN_SCRIPT}"; then
  fail "expected conformance defaults to bridge TCP listener ports 8080, 8090, 8443, and 8883"
fi

TEST_TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TEST_TMP_DIR}"' EXIT

cat >"${TEST_TMP_DIR}/ss" <<'EOF'
#!/usr/bin/env bash
requested=""
for arg in "$@"; do
  if [[ "${arg}" =~ sport[[:space:]]*=[[:space:]]*:([0-9]+) ]]; then
    requested="${BASH_REMATCH[1]}"
  fi
done
if [[ -z "${requested}" ]]; then
  cat "${SS_FIXTURE}"
else
  awk -v needle=":${requested}" '$0 ~ needle { print }' "${SS_FIXTURE}"
fi
EOF
chmod +x "${TEST_TMP_DIR}/ss"

cat >"${TEST_TMP_DIR}/iptables" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  *" -C "*)
    exit 1
    ;;
  *" -A "*)
    printf '%s\n' "$*" >>"${IPTABLES_LOG}"
    exit 0
    ;;
  *" -D "*)
    printf '%s\n' "$*" >>"${IPTABLES_LOG}"
    exit 0
    ;;
esac
exit 1
EOF
chmod +x "${TEST_TMP_DIR}/iptables"

cat >"${TEST_TMP_DIR}/socat" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"${SOCAT_LOG}"
if [[ "$1" =~ ^TCP-LISTEN:([0-9]+), ]]; then
  printf 'LISTEN 0 4096 127.0.0.1:%s 0.0.0.0:*\n' "${BASH_REMATCH[1]}" >>"${SS_FIXTURE}"
elif [[ "$1" =~ ^UDP-LISTEN:([0-9]+), ]]; then
  printf 'UNCONN 0 0 127.0.0.1:%s 0.0.0.0:*\n' "${BASH_REMATCH[1]}" >>"${SS_FIXTURE}"
fi
exit 1
EOF
chmod +x "${TEST_TMP_DIR}/socat"

CONFORMANCE_RUN_SH_SOURCE_ONLY=true source "${RUN_SCRIPT}"
export PATH="${TEST_TMP_DIR}:${PATH}"
IPTABLES_LOG="${TEST_TMP_DIR}/iptables.log"
SOCAT_LOG="${TEST_TMP_DIR}/socat.log"
export IPTABLES_LOG SOCAT_LOG

if [[ "$(shared_node_port_for 8883 TCP)" != "31883" ]]; then
  fail "expected shared TCP NodePort prediction for 8883 to stay inside the default NodePort range"
fi

SS_FIXTURE="${TEST_TMP_DIR}/tailscale-443.txt"
export SS_FIXTURE
cat >"${SS_FIXTURE}" <<'EOF'
LISTEN 0 4096 100.64.251.32:443 0.0.0.0:*
LISTEN 0 4096 [fd7a:115c:a1e0::f838:fb20]:443 [::]:*
EOF

if is_port_listening 443 TCP; then
  fail "expected non-loopback listeners to leave 127.0.0.1:443 available for relay setup"
fi

SS_FIXTURE="${TEST_TMP_DIR}/loopback-443.txt"
export SS_FIXTURE
cat >"${SS_FIXTURE}" <<'EOF'
LISTEN 0 4096 127.0.0.1:443 0.0.0.0:*
EOF

if ! is_port_listening 443 TCP; then
  fail "expected loopback listener to count as host port availability"
fi

SS_FIXTURE="${TEST_TMP_DIR}/wildcard-443.txt"
export SS_FIXTURE
cat >"${SS_FIXTURE}" <<'EOF'
LISTEN 0 4096 0.0.0.0:443 0.0.0.0:*
EOF

if ! is_port_listening 443 TCP; then
  fail "expected wildcard listener to count as host port availability"
fi

SS_FIXTURE="${TEST_TMP_DIR}/wildcard-8080.txt"
export SS_FIXTURE
cat >"${SS_FIXTURE}" <<'EOF'
LISTEN 0 4096 0.0.0.0:8080 0.0.0.0:*
EOF

HOST_PORT_RELAY_PIDS=()
HOST_PORT_REDIRECT_RULES=()
HOST_PORT_RELAYS=()
start_host_port_relay 8080 TCP 172.18.0.2 32080 "listener tcp"

if ! grep -Fq -- '-A OUTPUT -d 127.0.0.1/32 -p tcp -m tcp --dport 8080 -j DNAT --to-destination 127.0.0.1:20000' "${IPTABLES_LOG}"; then
  fail "expected occupied TCP listener port to install a localhost DNAT rule to a local relay port"
fi

if ! grep -Fq -- 'TCP-LISTEN:20000,bind=127.0.0.1,reuseaddr,fork TCP:172.18.0.2:32080' "${SOCAT_LOG}"; then
  fail "expected occupied TCP listener port to start a local relay to the kind NodePort"
fi

SS_FIXTURE="${TEST_TMP_DIR}/docker-proxy-5300.txt"
export SS_FIXTURE
cat >"${SS_FIXTURE}" <<'EOF'
UNCONN 0 0 0.0.0.0:5300 0.0.0.0:* users:(("docker-proxy",pid=123,fd=7))
EOF
: >"${IPTABLES_LOG}"
: >"${SOCAT_LOG}"
HOST_PORT_RELAY_PIDS=()
HOST_PORT_REDIRECT_RULES=()
HOST_PORT_RELAYS=()
start_host_port_relay 5300 UDP 172.18.0.2 31300 "listener udp"

if [[ -s "${IPTABLES_LOG}" || -s "${SOCAT_LOG}" ]]; then
  fail "expected existing docker-proxy listener to be treated as the kind host port mapping"
fi

printf '[conformance-listener-relay-defaults-test] ok\n'
