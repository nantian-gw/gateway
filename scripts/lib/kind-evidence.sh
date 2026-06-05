#!/usr/bin/env bash

aeg_kind_log() {
  local prefix="$1"
  shift
  printf '[%s] %s\n' "${prefix}" "$*"
}

aeg_require_command() {
  local prefix="$1"
  local name="$2"

  if ! command -v "${name}" >/dev/null 2>&1; then
    aeg_kind_log "${prefix}" "missing required command: ${name}"
    exit 1
  fi
}

aeg_kind_ensure_stack() {
  local prefix="$1"
  local root_dir="$2"
  local http_host="$3"
  local gateway_host_port="$4"
  local smoke_path="${5:-/}"

  if curl -fsS -H "Host: ${http_host}" "http://127.0.0.1:${gateway_host_port}${smoke_path}" >/dev/null 2>&1; then
    return
  fi

  aeg_kind_log "${prefix}" "smoke endpoint unavailable; refreshing kind stack"
  (
    cd "${root_dir}"
    SKIP_BUILD=true ./tests/e2e/run-kind.sh
  )
}

aeg_kind_collect_admin_snapshots() {
  local root_dir="$1"
  local output_dir="$2"
  local label="$3"

  ENABLE_KIND_PORT_FORWARD=true \
  INCLUDE_CONTROLPLANE_METRICS=false \
  OUTPUT_DIR="${output_dir}/${label}" \
  "${root_dir}/scripts/collect-admin-snapshots.sh"
}

aeg_kind_capture_resource_snapshot() {
  local output_dir="$1"
  local label="$2"
  local kind_context="${3:-kind-aether-gateway}"
  local kube_namespace="${4:-aether-gateway}"
  local output="${output_dir}/resources/${label}.tsv"
  local component
  local pod

  mkdir -p "${output_dir}/resources"
  {
    printf 'component\tpod\tfd_count\trss_kib\tthreads\n'
    for component in controlplane dataplane; do
      while read -r pod; do
        [[ -n "${pod}" ]] || continue
        kubectl --context "${kind_context}" -n "${kube_namespace}" exec "${pod}" -- sh -c '
          fd_count="$(ls /proc/1/fd | wc -l)"
          rss_kib="$(awk "/VmRSS:/ {print \$2}" /proc/1/status)"
          threads="$(awk "/Threads:/ {print \$2}" /proc/1/status)"
          printf "%s\t%s\t%s\n" "${fd_count}" "${rss_kib}" "${threads}"
        ' | awk -v component="${component}" -v pod="${pod}" 'BEGIN { FS="\t"; OFS="\t" } { print component, pod, $1, $2, $3 }'
      done < <(
        kubectl --context "${kind_context}" -n "${kube_namespace}" get pods \
          -l "app=aether-gateway-${component}" \
          -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}'
      )
    done
  } >"${output}"
}

aeg_kind_start_background_http_traffic() {
  local pid_var="$1"
  local http_client="$2"
  local output_dir="$3"
  local gateway_host_port="$4"
  local http_host="$5"
  local batch_requests="$6"
  local batch_concurrency="$7"
  local request_path="${8:-/}"
  local request_timeout="${9:-10}"
  local expect_body_substring="${10:-aether-gateway-ok}"

  mkdir -p "${output_dir}/traffic"
  (
    while true; do
      python3 "${http_client}" \
        --url "http://127.0.0.1:${gateway_host_port}${request_path}" \
        --host-header "${http_host}" \
        --requests "${batch_requests}" \
        --concurrency "${batch_concurrency}" \
        --connect-timeout 3 \
        --request-timeout "${request_timeout}" \
        --expect-status 200 \
        --expect-body-substring "${expect_body_substring}" \
        >>"${output_dir}/traffic/http-batches.jsonl"
    done
  ) &
  printf -v "${pid_var}" '%s' "$!"
}

aeg_kind_stop_background_pid() {
  local pid="${1:-}"

  if [[ -n "${pid}" ]]; then
    kill "${pid}" >/dev/null 2>&1 || true
    wait "${pid}" >/dev/null 2>&1 || true
  fi
}

aeg_kind_write_traffic_slo_gate() {
  local input="$1"
  local output="$2"
  local min_success_rate="$3"
  local max_errors="$4"
  local max_p99_ms="$5"
  local max_latency_ms="$6"
  local risk_accepted="$7"

  python3 - \
    "${input}" \
    "${output}" \
    "${min_success_rate}" \
    "${max_errors}" \
    "${max_p99_ms}" \
    "${max_latency_ms}" \
    "${risk_accepted}" <<'PY'
import json
import sys
from pathlib import Path

src = Path(sys.argv[1])
dst = Path(sys.argv[2])
min_success_rate = float(sys.argv[3])
max_errors = int(sys.argv[4])
max_p99_ms = float(sys.argv[5])
max_latency_ms = float(sys.argv[6])
risk_accepted = sys.argv[7].lower() in {"1", "true", "yes", "y"}

payload = json.loads(src.read_text(encoding="utf-8"))
completed = int(payload.get("completed", payload.get("requests", 0)) or 0)
requested = int(payload.get("requests", completed) or completed)
successes = int(payload.get("successes", 0) or 0)
errors = max(requested - successes, 0)
success_rate = payload.get("mean_success_rate", payload.get("success_rate"))
if success_rate is None:
    success_rate = successes / requested if requested else 0.0
success_rate = float(success_rate)

latency = payload.get("latency_ms", {})
p99_ms = float(payload.get("max_p99_ms", latency.get("p99", 0)) or 0)
max_observed_latency_ms = float(payload.get("max_latency_ms", latency.get("max", 0)) or 0)

checks = {
    "min_success_rate": success_rate >= min_success_rate,
    "max_errors": errors <= max_errors,
    "max_p99_ms": p99_ms <= max_p99_ms,
    "max_latency_ms": max_observed_latency_ms <= max_latency_ms,
}
violations = [name for name, ok in checks.items() if not ok]
status = "pass"
if violations:
    status = "risk-accepted" if risk_accepted else "fail"

payload["errors"] = errors
payload["slo_gate"] = {
    "status": status,
    "thresholds": {
        "min_success_rate": min_success_rate,
        "max_errors": max_errors,
        "max_p99_ms": max_p99_ms,
        "max_latency_ms": max_latency_ms,
    },
    "observed": {
        "success_rate": success_rate,
        "errors": errors,
        "p99_ms": p99_ms,
        "max_latency_ms": max_observed_latency_ms,
    },
    "violations": violations,
}

dst.write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY
}

aeg_kind_write_profile_slo_gate() {
  local output="$1"
  local min_success_rate="$2"
  local max_errors="$3"
  local max_latency_ms="$4"
  local risk_accepted="$5"
  shift 5

  python3 - \
    "${output}" \
    "${min_success_rate}" \
    "${max_errors}" \
    "${max_latency_ms}" \
    "${risk_accepted}" \
    "$@" <<'PY'
import json
import sys
from pathlib import Path

dst = Path(sys.argv[1])
min_success_rate = float(sys.argv[2])
max_errors = int(sys.argv[3])
max_latency_ms = float(sys.argv[4])
risk_accepted = sys.argv[5].lower() in {"1", "true", "yes", "y"}
specs = sys.argv[6:]

profiles = {}
overall_status = "pass"
for spec in specs:
    try:
        label, p99_threshold, path = spec.split(":", 2)
    except ValueError as exc:
        raise SystemExit(f"invalid profile spec {spec!r}; expected label:max_p99_ms:path") from exc
    payload = json.loads(Path(path).read_text(encoding="utf-8"))
    completed = int(payload.get("completed", payload.get("requests", 0)) or 0)
    requested = int(payload.get("requests", completed) or completed)
    successes = int(payload.get("successes", 0) or 0)
    errors = max(requested - successes, 0)
    success_rate = payload.get("mean_success_rate", payload.get("success_rate"))
    if success_rate is None:
        success_rate = successes / requested if requested else 0.0
    success_rate = float(success_rate)
    latency = payload.get("latency_ms", {})
    p99_ms = float(payload.get("max_p99_ms", latency.get("p99", 0)) or 0)
    max_observed_latency_ms = float(payload.get("max_latency_ms", latency.get("max", 0)) or 0)
    max_p99_ms = float(p99_threshold)

    checks = {
        "min_success_rate": success_rate >= min_success_rate,
        "max_errors": errors <= max_errors,
        "max_p99_ms": p99_ms <= max_p99_ms,
        "max_latency_ms": max_observed_latency_ms <= max_latency_ms,
    }
    violations = [name for name, ok in checks.items() if not ok]
    status = "pass"
    if violations:
        status = "risk-accepted" if risk_accepted else "fail"
    if status == "fail":
        overall_status = "fail"
    elif status == "risk-accepted" and overall_status == "pass":
        overall_status = "risk-accepted"

    profiles[label] = {
        "status": status,
        "source": path,
        "thresholds": {
            "min_success_rate": min_success_rate,
            "max_errors": max_errors,
            "max_p99_ms": max_p99_ms,
            "max_latency_ms": max_latency_ms,
        },
        "observed": {
            "success_rate": success_rate,
            "errors": errors,
            "p99_ms": p99_ms,
            "max_latency_ms": max_observed_latency_ms,
        },
        "violations": violations,
    }

summary = {
    "status": overall_status,
    "thresholds": {
        "min_success_rate": min_success_rate,
        "max_errors": max_errors,
        "max_latency_ms": max_latency_ms,
    },
    "profiles": {name: profiles[name] for name in sorted(profiles)},
}

dst.write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY
}

aeg_kind_slo_status() {
  local path="$1"

  python3 - "${path}" <<'PY'
import json
import sys
from pathlib import Path

payload = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
if "slo_gate" in payload:
    print(payload["slo_gate"].get("status", "unknown"))
else:
    print(payload.get("status", "unknown"))
PY
}
