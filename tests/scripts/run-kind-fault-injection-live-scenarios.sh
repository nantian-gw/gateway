#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="${ROOT_DIR}/scripts/run-kind-fault-injection.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

fail() {
  printf '[run-kind-fault-injection-live-test] %s\n' "$*" >&2
  exit 1
}

TEST_BIN="${TMP_DIR}/bin"
STATE_DIR="${TMP_DIR}/state"
OUTPUT_DIR="${TMP_DIR}/faults-live"
mkdir -p "${TEST_BIN}" "${STATE_DIR}"

cat >"${TEST_BIN}/python3" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == "-" ]]; then
  exec /usr/bin/python3 "$@"
fi

if [[ "${1:-}" == *"http_concurrency_client.py" ]]; then
  printf '%s\n' '{"requests":100,"completed":100,"successes":100,"success_rate":1.0,"latency_ms":{"p95":20,"p99":40,"p999":80,"max":100},"error_counts":{},"body_mismatches":0}'
  /usr/bin/sleep 0.02
  exit 0
fi

exec /usr/bin/python3 "$@"
EOF
chmod +x "${TEST_BIN}/python3"

cat >"${TEST_BIN}/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

url="${*: -1}"
case "${url}" in
  */livez|*/readyz|http://127.0.0.1:18080/*)
    printf 'ok\n'
    ;;
  */metrics)
    printf 'nantian_gateway_dataplane_xds_stream_failures_total 0\n'
    ;;
  *)
    printf '{}\n'
    ;;
esac
EOF
chmod +x "${TEST_BIN}/curl"

cat >"${TEST_BIN}/sleep" <<'EOF'
#!/usr/bin/env bash
/usr/bin/sleep 0.01
EOF
chmod +x "${TEST_BIN}/sleep"

cat >"${TEST_BIN}/kubectl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

state_dir="${FAKE_KUBE_STATE_DIR:?}"
printf '%s\n' "$*" >>"${state_dir}/kubectl-calls.log"

args="$*"
case "${args}" in
  *"port-forward"*)
    /usr/bin/sleep 60
    ;;
  *"get lease nantian-controlplane-leader"*)
    if [[ -f "${state_dir}/leader-switched" ]]; then
      printf 'cp-b_uid'
    else
      printf 'cp-a_uid'
    fi
    ;;
  *"delete pod cp-a"*)
    touch "${state_dir}/leader-switched"
    ;;
  *"get pods -l app=nantian-dataplane -o json")
    if [[ -f "${state_dir}/dataplane-deleted" ]]; then
      count_file="${state_dir}/dataplane-replacement-checks"
      count=0
      [[ -f "${count_file}" ]] && count="$(cat "${count_file}")"
      count=$((count + 1))
      printf '%s\n' "${count}" >"${count_file}"
      if [[ "${count}" -ge 2 ]]; then
        touch "${state_dir}/dataplane-replacement-ready"
      fi
    fi
    if [[ -f "${state_dir}/dataplane-replacement-ready" ]]; then
      cat <<'JSON'
{
  "items": [
    {
      "metadata": {"name": "dp-b"},
      "status": {"phase": "Running", "containerStatuses": [{"ready": true}]}
    },
    {
      "metadata": {"name": "dp-c"},
      "status": {"phase": "Running", "containerStatuses": [{"ready": true}]}
    }
  ]
}
JSON
    else
      cat <<'JSON'
{
  "items": [
    {
      "metadata": {"name": "dp-a", "deletionTimestamp": "2026-05-14T04:00:00Z"},
      "status": {"phase": "Running", "containerStatuses": [{"ready": true}]}
    },
    {
      "metadata": {"name": "dp-b"},
      "status": {"phase": "Running", "containerStatuses": [{"ready": true}]}
    },
    {
      "metadata": {"name": "dp-c"},
      "status": {"phase": "Pending", "containerStatuses": [{"ready": false}]}
    }
  ]
}
JSON
    fi
    ;;
  *"get pods -l app=nantian-dataplane"*)
    printf 'dp-a'
    ;;
  *"delete pod dp-a"*)
    touch "${state_dir}/dataplane-deleted"
    ;;
  *"get deploy/nantian-dataplane -o jsonpath="*)
    printf '2'
    ;;
  *"rollout status"*)
    ;;
  *"get nodes -o json"*)
    cat <<'JSON'
{
  "items": [
    {
      "metadata": {
        "name": "nantian-gw-control-plane",
        "labels": {"node-role.kubernetes.io/control-plane": ""}
      },
      "status": {"conditions": [{"type": "Ready", "status": "True"}]}
    },
    {
      "metadata": {
        "name": "worker-a",
        "labels": {}
      },
      "status": {"conditions": [{"type": "Ready", "status": "True"}]}
    },
    {
      "metadata": {
        "name": "worker-b",
        "labels": {}
      },
      "status": {"conditions": [{"type": "Ready", "status": "True"}]}
    }
  ]
}
JSON
    ;;
  *"drain worker-a"*)
    [[ -f "${state_dir}/dataplane-replacement-ready" ]] \
      || { printf 'node drain started before dataplane replacement was ready\n' >&2; exit 1; }
    touch "${state_dir}/node-drained"
    ;;
  *"uncordon worker-a"*)
    touch "${state_dir}/node-uncordoned"
    ;;
  *"scale deployment/echo"*)
    touch "${state_dir}/echo-backend-prepared"
    ;;
  *"patch deployment/echo --type=json"*)
    touch "${state_dir}/echo-backend-cleaned"
    ;;
  *"get pods -l app=echo"*)
    cat <<'JSON'
{
  "items": [
    {
      "spec": {"nodeName": "worker-a"},
      "status": {
        "phase": "Running",
        "containerStatuses": [{"ready": true}]
      }
    },
    {
      "spec": {"nodeName": "worker-b"},
      "status": {
        "phase": "Running",
        "containerStatuses": [{"ready": true}]
      }
    }
  ]
}
JSON
    ;;
  *"get gateways.gateway.networking.k8s.io -A -o json"*)
    cat <<'JSON'
{"items":[{"metadata":{"namespace":"default","name":"smoke"}}]}
JSON
    ;;
  *"get httproutes.gateway.networking.k8s.io -A -o json"*)
    cat <<'JSON'
{"items":[{"metadata":{"namespace":"default","name":"echo"}}]}
JSON
    ;;
  *"patch gateways.gateway.networking.k8s.io"*)
    touch "${state_dir}/gateway-patched"
    ;;
  *"patch httproutes.gateway.networking.k8s.io"*)
    touch "${state_dir}/httproute-patched"
    ;;
  *"logs deploy/nantian-controlplane"*)
    printf 'controlplane log\n'
    ;;
  *"logs deploy/nantian-dataplane"*)
    printf 'dataplane log\n'
    ;;
  *)
    printf 'unexpected kubectl command: %s\n' "${args}" >&2
    exit 1
    ;;
esac
EOF
chmod +x "${TEST_BIN}/kubectl"

PATH="${TEST_BIN}:/usr/bin:/bin" \
FAKE_KUBE_STATE_DIR="${STATE_DIR}" \
OUTPUT_DIR="${OUTPUT_DIR}" \
RUN_ID="fixture-kind-faults-live" \
TRAFFIC_BATCH_REQUESTS=100 \
TRAFFIC_BATCH_CONCURRENCY=8 \
APISERVER_WATCH_CHURN_ITERATIONS=2 \
NODE_DRAIN_TIMEOUT=30s \
SKIP_NODE_DRAIN=false \
SKIP_APISERVER_WATCH_DISRUPTION=false \
REQUIRE_RELEASE_GATE_CONCLUSIONS=true \
timeout 15s bash "${SCRIPT}" >"${TMP_DIR}/stdout.log" 2>"${TMP_DIR}/stderr.log" \
  || {
    cat "${TMP_DIR}/stdout.log" >&2 || true
    cat "${TMP_DIR}/stderr.log" >&2 || true
    fail "expected live fake run to pass"
  }

[[ -f "${STATE_DIR}/node-drained" ]] || fail "expected worker node to be drained"
[[ -f "${STATE_DIR}/node-uncordoned" ]] || fail "expected worker node to be uncordoned"
[[ -f "${STATE_DIR}/dataplane-replacement-ready" ]] || fail "expected dataplane replacement wait before node drain"
[[ -f "${STATE_DIR}/echo-backend-prepared" ]] || fail "expected HTTP backend to be prepared for node drain"
[[ -f "${STATE_DIR}/gateway-patched" ]] || fail "expected Gateway watch churn patch"
[[ -f "${STATE_DIR}/httproute-patched" ]] || fail "expected HTTPRoute watch churn patch"

python3 - "${OUTPUT_DIR}" <<'PY'
import json
import sys
from pathlib import Path

root = Path(sys.argv[1])
conclusions = json.loads((root / "conclusions" / "summary.json").read_text(encoding="utf-8"))
assert conclusions["release_gate_status"] == "pass"
assert conclusions["missing_required_scenarios"] == []
assert conclusions["scenarios"]["node-drain"]["status"] == "pass"
assert "worker-a" in conclusions["scenarios"]["node-drain"]["summary"]
assert conclusions["scenarios"]["apiserver-watch-disruption"]["status"] == "pass"
assert "watch churn" in conclusions["scenarios"]["apiserver-watch-disruption"]["summary"]
traffic = json.loads((root / "traffic" / "summary.json").read_text(encoding="utf-8"))
assert traffic["slo_gate"]["status"] == "pass"
PY

grep -q '| node-drain | pass |' "${OUTPUT_DIR}/summary.md" \
  || fail "expected node-drain row in summary"
grep -q '| apiserver-watch-disruption | pass |' "${OUTPUT_DIR}/summary.md" \
  || fail "expected apiserver-watch-disruption row in summary"

printf '[run-kind-fault-injection-live-test] ok\n'
