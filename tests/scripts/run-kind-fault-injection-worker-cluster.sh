#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="${ROOT_DIR}/scripts/run-kind-fault-injection.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

fail() {
  printf '[run-kind-fault-injection-worker-cluster-test] %s\n' "$*" >&2
  exit 1
}

TEST_BIN="${TMP_DIR}/bin"
STATE_DIR="${TMP_DIR}/state"
OUTPUT_DIR="${TMP_DIR}/faults-worker-cluster"
RUN_KIND_STUB="${TMP_DIR}/run-kind.sh"
mkdir -p "${TEST_BIN}" "${STATE_DIR}" "${OUTPUT_DIR}"

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

cat >"${RUN_KIND_STUB}" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

state_dir="${FAKE_KUBE_STATE_DIR:?}"
{
  printf 'RECREATE_CLUSTER=%s\n' "${RECREATE_CLUSTER:-}"
  printf 'KIND_WORKER_NODES=%s\n' "${KIND_WORKER_NODES:-}"
  printf 'SKIP_BUILD=%s\n' "${SKIP_BUILD:-}"
} >"${state_dir}/run-kind-env.log"
touch "${state_dir}/worker-cluster-ready"
EOF
chmod +x "${RUN_KIND_STUB}"

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
      touch "${state_dir}/dataplane-replacement-ready"
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
      "metadata": {"name": "dp-a"},
      "status": {"phase": "Running", "containerStatuses": [{"ready": true}]}
    },
    {
      "metadata": {"name": "dp-b"},
      "status": {"phase": "Running", "containerStatuses": [{"ready": true}]}
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
    if [[ -f "${state_dir}/worker-cluster-ready" ]]; then
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
    else
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
    }
  ]
}
JSON
    fi
    ;;
  *"drain worker-a"*)
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
  *"delete pod -l nantian.dev/fault-traffic-guard=true"*)
    rm -f "${state_dir}/echo-fault-guard-created"
    ;;
  *"get deployment/echo -o jsonpath="*)
    printf 'localhost:5001/hashicorp/http-echo:1.0.0'
    ;;
  *"apply -f -"*)
    manifest="$(cat)"
    grep -q 'name: echo-fault-traffic-worker-a' <<<"${manifest}" \
      || { printf 'expected echo fault guard pod name in manifest\n' >&2; exit 1; }
    grep -q 'nodeName: worker-a' <<<"${manifest}" \
      || { printf 'expected echo fault guard pod to target worker-a\n' >&2; exit 1; }
    touch "${state_dir}/echo-fault-guard-created"
    ;;
  *"get pods -l app=echo"*)
    if [[ -f "${state_dir}/echo-fault-guard-created" ]]; then
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
    else
      cat <<'JSON'
{
  "items": [
    {
      "spec": {"nodeName": "worker-b"},
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
    fi
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
RUN_ID="fixture-kind-faults-worker-cluster" \
RUN_KIND_SCRIPT="${RUN_KIND_STUB}" \
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
    fail "expected release-gate fake run to recreate a worker cluster and pass"
  }

[[ -f "${STATE_DIR}/run-kind-env.log" ]] || fail "expected run-kind worker-cluster refresh"
grep -q '^RECREATE_CLUSTER=true$' "${STATE_DIR}/run-kind-env.log" \
  || fail "expected worker-cluster refresh to recreate kind"
grep -q '^KIND_WORKER_NODES=2$' "${STATE_DIR}/run-kind-env.log" \
  || fail "expected worker-cluster refresh to request two workers"
grep -q '^SKIP_BUILD=true$' "${STATE_DIR}/run-kind-env.log" \
  || fail "expected worker-cluster refresh to reuse built images"
[[ -f "${STATE_DIR}/node-drained" ]] || fail "expected recreated worker node to be drained"
[[ -f "${STATE_DIR}/node-uncordoned" ]] || fail "expected recreated worker node to be uncordoned"
[[ -f "${STATE_DIR}/dataplane-replacement-ready" ]] || fail "expected dataplane replacement readiness check"
[[ -f "${STATE_DIR}/echo-backend-prepared" ]] || fail "expected HTTP backend to be prepared"
[[ -f "${STATE_DIR}/echo-fault-guard-created" ]] || fail "expected HTTP backend guard pod to be created"

python3 - "${OUTPUT_DIR}" <<'PY'
import json
import sys
from pathlib import Path

root = Path(sys.argv[1])
conclusions = json.loads((root / "conclusions" / "summary.json").read_text(encoding="utf-8"))
assert conclusions["release_gate_status"] == "pass"
assert conclusions["scenarios"]["node-drain"]["status"] == "pass"
assert "worker-a" in conclusions["scenarios"]["node-drain"]["summary"]
PY

printf '[run-kind-fault-injection-worker-cluster-test] ok\n'
