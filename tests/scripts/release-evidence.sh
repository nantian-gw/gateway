#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CHECK_SCRIPT="${ROOT_DIR}/scripts/verify-release-evidence.sh"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

fail() {
  printf '[release-evidence-test] %s\n' "$*" >&2
  exit 1
}

write_conformance_metadata() {
  local path="$1"
  local implementation_version="$2"

  cat >"${path}" <<EOF
id: test-full-suite
scope: runs
result: passed
archivedAt: 2026-04-24T00:00:00Z
releaseTag: ""
sourceRef: "main"
sourceRunURL: ""
sourceCommand: |
  ALL_FEATURES=true ./tests/conformance/run.sh
report:
  file: report.yaml
  logFile: "run.log"
  reportDate: "2026-04-24T08:00:00+08:00"
  gatewayAPIChannel: "experimental"
  gatewayAPIVersion: "v1.4.1"
  implementationVersion: "${implementation_version}"
  mode: "default"
EOF
}

write_benchmark_metadata() {
  local path="$1"
  local git_commit="$2"
  local code_tree_state="${3:-clean}"

  cat >"${path}" <<EOF
captured_at=2026-04-24T08:00:00+08:00
git_commit=${git_commit}
git_tree_state=dirty
code_tree_state=${code_tree_state}
run_id=test-run
EOF

  if [[ "$(basename "${path}")" == "performance.txt" ]]; then
    write_performance_throughput_report "${path}"
    write_performance_source_slo_gate "${path}"
  fi
}

write_performance_throughput_report() {
  local metadata_path="$1"
  local missing_protocols="${2:-[]}"
  local missing_scenarios="${3:-[]}"
  local missing_reload_protocols="${4:-[]}"
  local missing_reload_mutations="${5:-[]}"

  local output_dir
  output_dir="$(dirname "${metadata_path}")"
  cat >"${output_dir}/throughput-report.json" <<EOF
{
  "coverage": {
    "missing_protocols": ${missing_protocols},
    "missing_scenarios": ${missing_scenarios}
  },
  "reload": {
    "live_traffic": {
      "missing_protocols": ${missing_reload_protocols},
      "missing_mutations": ${missing_reload_mutations}
    }
  }
}
EOF
}

write_performance_source_slo_gate() {
  local metadata_path="$1"
  local status="${2:-pass}"
  local violations="${3:-[]}"

  local output_dir
  output_dir="$(dirname "${metadata_path}")/source-kind-a4"
  mkdir -p "${output_dir}"
  cat >"${output_dir}/slo-gate.json" <<EOF
{
  "profiles": {
    "steady": {
      "observed": {
        "errors": 0,
        "max_latency_ms": 200.0,
        "p99_ms": 100.0,
        "success_rate": 1.0
      },
      "source": "http/steady.json",
      "status": "${status}",
      "thresholds": {
        "max_errors": 0,
        "max_latency_ms": 30000.0,
        "max_p99_ms": 500.0,
        "min_success_rate": 1.0
      },
      "violations": ${violations}
    }
  },
  "status": "${status}",
  "thresholds": {
    "max_errors": 0,
    "max_latency_ms": 30000.0,
    "min_success_rate": 1.0
  }
}
EOF
}

write_soak_metadata() {
  local path="$1"
  local git_commit="$2"
  local code_tree_state="${3:-clean}"
  local duration_seconds="${4:-86400}"

  write_benchmark_metadata "${path}" "${git_commit}" "${code_tree_state}"
  printf 'duration_seconds=%s\n' "${duration_seconds}" >>"${path}"
  write_soak_traffic_summary "${path}" "pass"
}

write_soak_traffic_summary() {
  local metadata_path="$1"
  local slo_status="${2:-pass}"

  local output_dir
  output_dir="$(dirname "${metadata_path}")/traffic"
  mkdir -p "${output_dir}"
  cat >"${output_dir}/summary.json" <<EOF
{
  "completed": 864000,
  "errors": 0,
  "max_latency_ms": 2090.71,
  "max_p99_ms": 1063.08,
  "mean_success_rate": 1.0,
  "slo_gate": {
    "observed": {
      "errors": 0,
      "max_latency_ms": 2090.71,
      "p99_ms": 1063.08,
      "success_rate": 1.0
    },
    "status": "${slo_status}",
    "thresholds": {
      "max_errors": 0,
      "max_latency_ms": 30000.0,
      "max_p99_ms": 5000.0,
      "min_success_rate": 1.0
    },
    "violations": []
  },
  "successes": 864000
}
EOF
}

write_chaos_conclusions_summary() {
  local metadata_path="$1"
  local release_gate_status="${2:-pass}"
  local missing_required="${3:-[]}"

  local output_dir
  output_dir="$(dirname "${metadata_path}")/conclusions"
  mkdir -p "${output_dir}"
  cat >"${output_dir}/summary.json" <<EOF
{
  "missing_required_scenarios": ${missing_required},
  "observed_scenarios": [
    "apiserver-watch-disruption",
    "controlplane-leader-switch",
    "dataplane-pod-restart",
    "node-drain"
  ],
  "release_gate_status": "${release_gate_status}",
  "required_scenarios": [
    "controlplane-leader-switch",
    "dataplane-pod-restart",
    "node-drain",
    "apiserver-watch-disruption"
  ],
  "scenarios": {},
  "status_counts": {
    "${release_gate_status}": 4
  }
}
EOF

  write_chaos_traffic_summary "${metadata_path}" "pass"
}

write_chaos_traffic_summary() {
  local metadata_path="$1"
  local slo_status="${2:-pass}"

  local output_dir
  output_dir="$(dirname "${metadata_path}")/traffic"
  mkdir -p "${output_dir}"
  cat >"${output_dir}/summary.json" <<EOF
{
  "completed": 32000,
  "errors": 0,
  "max_latency_ms": 2090.71,
  "max_p99_ms": 1063.08,
  "mean_success_rate": 1.0,
  "slo_gate": {
    "observed": {
      "errors": 0,
      "max_latency_ms": 2090.71,
      "p99_ms": 1063.08,
      "success_rate": 1.0
    },
    "status": "${slo_status}",
    "thresholds": {
      "max_errors": 0,
      "max_latency_ms": 30000.0,
      "max_p99_ms": 5000.0,
      "min_success_rate": 1.0
    },
    "violations": []
  },
  "successes": 32000
}
EOF
}

mkdir -p "${TMP_DIR}/matching" "${TMP_DIR}/dirty" "${TMP_DIR}/window"

candidate_short="66c7bef"
candidate_full="66c7bef0123456789abcdef0123456789abcdef"
window_short="af4a7d3"
window_full="af4a7d30123456789abcdef0123456789abcdef"

"${CHECK_SCRIPT}" --help >"${TMP_DIR}/help.txt"
grep -q 'release-candidate evidence' "${TMP_DIR}/help.txt" \
  || fail "expected --help output to describe release evidence validation"
grep -q -- '--allow-dirty-soak-code-tree' "${TMP_DIR}/help.txt" \
  || fail "expected --help output to include dirty soak code tree risk acceptance"
grep -q -- '--allow-dirty-chaos-code-tree' "${TMP_DIR}/help.txt" \
  || fail "expected --help output to include dirty chaos code tree risk acceptance"

if "${CHECK_SCRIPT}" --candidate >"${TMP_DIR}/missing-arg.stdout" 2>"${TMP_DIR}/missing-arg.stderr"; then
  fail "expected missing --candidate value to fail"
else
  status=$?
fi
[[ "${status}" -eq 2 ]] || fail "expected missing --candidate value to exit 2, got ${status}"

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/missing/conformance.yaml" \
  --performance "${TMP_DIR}/missing/performance.txt" \
  --chaos "${TMP_DIR}/missing/chaos.txt" \
  --soak "${TMP_DIR}/missing/soak.txt" \
  >"${TMP_DIR}/missing-file.stdout" 2>"${TMP_DIR}/missing-file.stderr"; then
  fail "expected missing evidence files to fail"
fi
grep -q 'required file not found' "${TMP_DIR}/missing-file.stderr" \
  || fail "expected missing evidence file failure to use required-file wording"

write_conformance_metadata \
  "${TMP_DIR}/matching/conformance.yaml" \
  "${candidate_short}"
write_benchmark_metadata \
  "${TMP_DIR}/matching/performance.txt" \
  "${candidate_full}" \
  "clean"
write_benchmark_metadata \
  "${TMP_DIR}/matching/chaos.txt" \
  "${candidate_full}" \
  "clean"
write_chaos_conclusions_summary "${TMP_DIR}/matching/chaos.txt"
write_soak_metadata \
  "${TMP_DIR}/matching/soak.txt" \
  "${candidate_full}" \
  "clean"

"${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/matching/conformance.yaml" \
  --performance "${TMP_DIR}/matching/performance.txt" \
  --chaos "${TMP_DIR}/matching/chaos.txt" \
  --soak "${TMP_DIR}/matching/soak.txt" \
  >"${TMP_DIR}/matching/stdout.log"

grep -q 'release evidence verified' "${TMP_DIR}/matching/stdout.log" \
  || fail "expected success summary for matching evidence"

write_conformance_metadata \
  "${TMP_DIR}/dirty/conformance.yaml" \
  "${candidate_short}-dirty"
write_benchmark_metadata \
  "${TMP_DIR}/dirty/performance.txt" \
  "${candidate_full}" \
  "clean"
write_benchmark_metadata \
  "${TMP_DIR}/dirty/chaos.txt" \
  "${candidate_full}" \
  "clean"
write_chaos_conclusions_summary "${TMP_DIR}/dirty/chaos.txt"
write_soak_metadata \
  "${TMP_DIR}/dirty/soak.txt" \
  "${candidate_full}" \
  "clean"

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/dirty/conformance.yaml" \
  --performance "${TMP_DIR}/dirty/performance.txt" \
  --chaos "${TMP_DIR}/dirty/chaos.txt" \
  --soak "${TMP_DIR}/dirty/soak.txt" \
  >"${TMP_DIR}/dirty/stdout.log" 2>"${TMP_DIR}/dirty/stderr.log"; then
  fail "expected dirty conformance evidence to be rejected"
fi

grep -q 'dirty' "${TMP_DIR}/dirty/stderr.log" \
  || fail "expected dirty rejection reason"

write_conformance_metadata \
  "${TMP_DIR}/window/conformance.yaml" \
  "${candidate_short}"
write_benchmark_metadata \
  "${TMP_DIR}/window/performance.txt" \
  "${window_full}" \
  "clean"
write_benchmark_metadata \
  "${TMP_DIR}/window/chaos.txt" \
  "${window_full}" \
  "clean"
write_chaos_conclusions_summary "${TMP_DIR}/window/chaos.txt"
write_soak_metadata \
  "${TMP_DIR}/window/soak.txt" \
  "${window_full}" \
  "clean"

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/window/conformance.yaml" \
  --performance "${TMP_DIR}/window/performance.txt" \
  --chaos "${TMP_DIR}/window/chaos.txt" \
  --soak "${TMP_DIR}/window/soak.txt" \
  >"${TMP_DIR}/window/stdout.log" 2>"${TMP_DIR}/window/stderr.log"; then
  fail "expected mismatched evidence commit to be rejected without allowed window"
fi

grep -q "${window_short}" "${TMP_DIR}/window/stderr.log" \
  || fail "expected mismatched commit to be reported"

"${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --allow-commit "${window_short}" \
  --conformance "${TMP_DIR}/window/conformance.yaml" \
  --performance "${TMP_DIR}/window/performance.txt" \
  --chaos "${TMP_DIR}/window/chaos.txt" \
  --soak "${TMP_DIR}/window/soak.txt" \
  >"${TMP_DIR}/window/allowed.log"

grep -q 'allowed commits' "${TMP_DIR}/window/allowed.log" \
  || fail "expected success summary to mention allowed commit window"

mkdir -p "${TMP_DIR}/dirty-code-tree"
write_conformance_metadata \
  "${TMP_DIR}/dirty-code-tree/conformance.yaml" \
  "${candidate_short}"
write_benchmark_metadata \
  "${TMP_DIR}/dirty-code-tree/performance.txt" \
  "${candidate_full}" \
  "dirty"
write_benchmark_metadata \
  "${TMP_DIR}/dirty-code-tree/chaos.txt" \
  "${candidate_full}" \
  "clean"
write_chaos_conclusions_summary "${TMP_DIR}/dirty-code-tree/chaos.txt"
write_soak_metadata \
  "${TMP_DIR}/dirty-code-tree/soak.txt" \
  "${candidate_full}" \
  "clean"

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/dirty-code-tree/conformance.yaml" \
  --performance "${TMP_DIR}/dirty-code-tree/performance.txt" \
  --chaos "${TMP_DIR}/dirty-code-tree/chaos.txt" \
  --soak "${TMP_DIR}/dirty-code-tree/soak.txt" \
  >"${TMP_DIR}/dirty-code-tree/stdout.log" 2>"${TMP_DIR}/dirty-code-tree/stderr.log"; then
  fail "expected dirty performance code tree to be rejected"
fi

grep -q 'performance evidence code_tree_state dirty' "${TMP_DIR}/dirty-code-tree/stderr.log" \
  || fail "expected dirty performance code tree rejection reason"

"${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --allow-dirty-performance-code-tree \
  --conformance "${TMP_DIR}/dirty-code-tree/conformance.yaml" \
  --performance "${TMP_DIR}/dirty-code-tree/performance.txt" \
  --chaos "${TMP_DIR}/dirty-code-tree/chaos.txt" \
  --soak "${TMP_DIR}/dirty-code-tree/soak.txt" \
  >"${TMP_DIR}/dirty-code-tree/allowed.log"

grep -q 'performance code_tree_state: dirty (risk-accepted)' "${TMP_DIR}/dirty-code-tree/allowed.log" \
  || fail "expected accepted dirty performance code tree to be reported"

mkdir -p "${TMP_DIR}/missing-code-tree"
write_conformance_metadata \
  "${TMP_DIR}/missing-code-tree/conformance.yaml" \
  "${candidate_short}"
cat >"${TMP_DIR}/missing-code-tree/performance.txt" <<EOF
captured_at=2026-04-24T08:00:00+08:00
git_commit=${candidate_full}
git_tree_state=dirty
run_id=test-run
EOF
write_performance_throughput_report "${TMP_DIR}/missing-code-tree/performance.txt"
write_performance_source_slo_gate "${TMP_DIR}/missing-code-tree/performance.txt"
write_benchmark_metadata \
  "${TMP_DIR}/missing-code-tree/chaos.txt" \
  "${candidate_full}" \
  "clean"
write_chaos_conclusions_summary "${TMP_DIR}/missing-code-tree/chaos.txt"
write_soak_metadata \
  "${TMP_DIR}/missing-code-tree/soak.txt" \
  "${candidate_full}" \
  "clean"

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/missing-code-tree/conformance.yaml" \
  --performance "${TMP_DIR}/missing-code-tree/performance.txt" \
  --chaos "${TMP_DIR}/missing-code-tree/chaos.txt" \
  --soak "${TMP_DIR}/missing-code-tree/soak.txt" \
  >"${TMP_DIR}/missing-code-tree/stdout.log" 2>"${TMP_DIR}/missing-code-tree/stderr.log"; then
  fail "expected missing performance code_tree_state to be rejected"
fi

grep -q 'missing code_tree_state' "${TMP_DIR}/missing-code-tree/stderr.log" \
  || fail "expected missing code_tree_state rejection reason"

"${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --allow-dirty-performance-code-tree \
  --conformance "${TMP_DIR}/missing-code-tree/conformance.yaml" \
  --performance "${TMP_DIR}/missing-code-tree/performance.txt" \
  --chaos "${TMP_DIR}/missing-code-tree/chaos.txt" \
  --soak "${TMP_DIR}/missing-code-tree/soak.txt" \
  >"${TMP_DIR}/missing-code-tree/allowed.log"

grep -q 'performance code_tree_state: missing (risk-accepted)' "${TMP_DIR}/missing-code-tree/allowed.log" \
  || fail "expected accepted missing performance code tree to be reported"

mkdir -p "${TMP_DIR}/missing-performance-throughput-report"
write_conformance_metadata \
  "${TMP_DIR}/missing-performance-throughput-report/conformance.yaml" \
  "${candidate_short}"
write_benchmark_metadata \
  "${TMP_DIR}/missing-performance-throughput-report/performance.txt" \
  "${candidate_full}" \
  "clean"
rm -f "${TMP_DIR}/missing-performance-throughput-report/throughput-report.json"
write_benchmark_metadata \
  "${TMP_DIR}/missing-performance-throughput-report/chaos.txt" \
  "${candidate_full}" \
  "clean"
write_chaos_conclusions_summary "${TMP_DIR}/missing-performance-throughput-report/chaos.txt"
write_soak_metadata \
  "${TMP_DIR}/missing-performance-throughput-report/soak.txt" \
  "${candidate_full}" \
  "clean"

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/missing-performance-throughput-report/conformance.yaml" \
  --performance "${TMP_DIR}/missing-performance-throughput-report/performance.txt" \
  --chaos "${TMP_DIR}/missing-performance-throughput-report/chaos.txt" \
  --soak "${TMP_DIR}/missing-performance-throughput-report/soak.txt" \
  >"${TMP_DIR}/missing-performance-throughput-report/stdout.log" 2>"${TMP_DIR}/missing-performance-throughput-report/stderr.log"; then
  fail "expected missing performance throughput report to be rejected"
fi

grep -q 'performance evidence is missing throughput report' "${TMP_DIR}/missing-performance-throughput-report/stderr.log" \
  || fail "expected missing performance throughput report rejection reason"

mkdir -p "${TMP_DIR}/missing-performance-protocol"
write_conformance_metadata \
  "${TMP_DIR}/missing-performance-protocol/conformance.yaml" \
  "${candidate_short}"
write_benchmark_metadata \
  "${TMP_DIR}/missing-performance-protocol/performance.txt" \
  "${candidate_full}" \
  "clean"
write_performance_throughput_report \
  "${TMP_DIR}/missing-performance-protocol/performance.txt" \
  '["udp"]'
write_benchmark_metadata \
  "${TMP_DIR}/missing-performance-protocol/chaos.txt" \
  "${candidate_full}" \
  "clean"
write_chaos_conclusions_summary "${TMP_DIR}/missing-performance-protocol/chaos.txt"
write_soak_metadata \
  "${TMP_DIR}/missing-performance-protocol/soak.txt" \
  "${candidate_full}" \
  "clean"

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/missing-performance-protocol/conformance.yaml" \
  --performance "${TMP_DIR}/missing-performance-protocol/performance.txt" \
  --chaos "${TMP_DIR}/missing-performance-protocol/chaos.txt" \
  --soak "${TMP_DIR}/missing-performance-protocol/soak.txt" \
  >"${TMP_DIR}/missing-performance-protocol/stdout.log" 2>"${TMP_DIR}/missing-performance-protocol/stderr.log"; then
  fail "expected incomplete performance protocol coverage to be rejected"
fi

grep -q 'performance evidence coverage missing_protocols is not empty' "${TMP_DIR}/missing-performance-protocol/stderr.log" \
  || fail "expected missing performance protocol rejection reason"

mkdir -p "${TMP_DIR}/missing-performance-reload-mutation"
write_conformance_metadata \
  "${TMP_DIR}/missing-performance-reload-mutation/conformance.yaml" \
  "${candidate_short}"
write_benchmark_metadata \
  "${TMP_DIR}/missing-performance-reload-mutation/performance.txt" \
  "${candidate_full}" \
  "clean"
write_performance_throughput_report \
  "${TMP_DIR}/missing-performance-reload-mutation/performance.txt" \
  '[]' \
  '[]' \
  '[]' \
  '["endpoint-change"]'
write_benchmark_metadata \
  "${TMP_DIR}/missing-performance-reload-mutation/chaos.txt" \
  "${candidate_full}" \
  "clean"
write_chaos_conclusions_summary "${TMP_DIR}/missing-performance-reload-mutation/chaos.txt"
write_soak_metadata \
  "${TMP_DIR}/missing-performance-reload-mutation/soak.txt" \
  "${candidate_full}" \
  "clean"

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/missing-performance-reload-mutation/conformance.yaml" \
  --performance "${TMP_DIR}/missing-performance-reload-mutation/performance.txt" \
  --chaos "${TMP_DIR}/missing-performance-reload-mutation/chaos.txt" \
  --soak "${TMP_DIR}/missing-performance-reload-mutation/soak.txt" \
  >"${TMP_DIR}/missing-performance-reload-mutation/stdout.log" 2>"${TMP_DIR}/missing-performance-reload-mutation/stderr.log"; then
  fail "expected incomplete performance reload mutation coverage to be rejected"
fi

grep -q 'performance evidence reload live traffic missing_mutations is not empty' "${TMP_DIR}/missing-performance-reload-mutation/stderr.log" \
  || fail "expected missing performance reload mutation rejection reason"

mkdir -p "${TMP_DIR}/missing-performance-source-slo"
write_conformance_metadata \
  "${TMP_DIR}/missing-performance-source-slo/conformance.yaml" \
  "${candidate_short}"
write_benchmark_metadata \
  "${TMP_DIR}/missing-performance-source-slo/performance.txt" \
  "${candidate_full}" \
  "clean"
rm -rf "${TMP_DIR}/missing-performance-source-slo/source-kind-a4"
write_benchmark_metadata \
  "${TMP_DIR}/missing-performance-source-slo/chaos.txt" \
  "${candidate_full}" \
  "clean"
write_chaos_conclusions_summary "${TMP_DIR}/missing-performance-source-slo/chaos.txt"
write_soak_metadata \
  "${TMP_DIR}/missing-performance-source-slo/soak.txt" \
  "${candidate_full}" \
  "clean"

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/missing-performance-source-slo/conformance.yaml" \
  --performance "${TMP_DIR}/missing-performance-source-slo/performance.txt" \
  --chaos "${TMP_DIR}/missing-performance-source-slo/chaos.txt" \
  --soak "${TMP_DIR}/missing-performance-source-slo/soak.txt" \
  >"${TMP_DIR}/missing-performance-source-slo/stdout.log" 2>"${TMP_DIR}/missing-performance-source-slo/stderr.log"; then
  fail "expected missing performance source SLO gate to be rejected"
fi

grep -q 'performance evidence is missing source SLO gate' "${TMP_DIR}/missing-performance-source-slo/stderr.log" \
  || fail "expected missing performance source SLO gate rejection reason"

mkdir -p "${TMP_DIR}/failed-performance-source-slo"
write_conformance_metadata \
  "${TMP_DIR}/failed-performance-source-slo/conformance.yaml" \
  "${candidate_short}"
write_benchmark_metadata \
  "${TMP_DIR}/failed-performance-source-slo/performance.txt" \
  "${candidate_full}" \
  "clean"
write_performance_source_slo_gate \
  "${TMP_DIR}/failed-performance-source-slo/performance.txt" \
  "fail" \
  '["steady p99 exceeded"]'
write_benchmark_metadata \
  "${TMP_DIR}/failed-performance-source-slo/chaos.txt" \
  "${candidate_full}" \
  "clean"
write_chaos_conclusions_summary "${TMP_DIR}/failed-performance-source-slo/chaos.txt"
write_soak_metadata \
  "${TMP_DIR}/failed-performance-source-slo/soak.txt" \
  "${candidate_full}" \
  "clean"

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/failed-performance-source-slo/conformance.yaml" \
  --performance "${TMP_DIR}/failed-performance-source-slo/performance.txt" \
  --chaos "${TMP_DIR}/failed-performance-source-slo/chaos.txt" \
  --soak "${TMP_DIR}/failed-performance-source-slo/soak.txt" \
  >"${TMP_DIR}/failed-performance-source-slo/stdout.log" 2>"${TMP_DIR}/failed-performance-source-slo/stderr.log"; then
  fail "expected failed performance source SLO gate to be rejected"
fi

grep -q 'performance evidence source SLO gate fail is not pass' "${TMP_DIR}/failed-performance-source-slo/stderr.log" \
  || fail "expected failed performance source SLO gate rejection reason"

mkdir -p "${TMP_DIR}/dirty-chaos-code-tree"
write_conformance_metadata \
  "${TMP_DIR}/dirty-chaos-code-tree/conformance.yaml" \
  "${candidate_short}"
write_benchmark_metadata \
  "${TMP_DIR}/dirty-chaos-code-tree/performance.txt" \
  "${candidate_full}" \
  "clean"
write_benchmark_metadata \
  "${TMP_DIR}/dirty-chaos-code-tree/chaos.txt" \
  "${candidate_full}" \
  "dirty"
write_chaos_conclusions_summary "${TMP_DIR}/dirty-chaos-code-tree/chaos.txt"
write_soak_metadata \
  "${TMP_DIR}/dirty-chaos-code-tree/soak.txt" \
  "${candidate_full}" \
  "clean"

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/dirty-chaos-code-tree/conformance.yaml" \
  --performance "${TMP_DIR}/dirty-chaos-code-tree/performance.txt" \
  --chaos "${TMP_DIR}/dirty-chaos-code-tree/chaos.txt" \
  --soak "${TMP_DIR}/dirty-chaos-code-tree/soak.txt" \
  >"${TMP_DIR}/dirty-chaos-code-tree/stdout.log" 2>"${TMP_DIR}/dirty-chaos-code-tree/stderr.log"; then
  fail "expected dirty chaos code tree to be rejected"
fi

grep -q 'chaos evidence code_tree_state dirty' "${TMP_DIR}/dirty-chaos-code-tree/stderr.log" \
  || fail "expected dirty chaos code tree rejection reason"

"${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --allow-dirty-chaos-code-tree \
  --conformance "${TMP_DIR}/dirty-chaos-code-tree/conformance.yaml" \
  --performance "${TMP_DIR}/dirty-chaos-code-tree/performance.txt" \
  --chaos "${TMP_DIR}/dirty-chaos-code-tree/chaos.txt" \
  --soak "${TMP_DIR}/dirty-chaos-code-tree/soak.txt" \
  >"${TMP_DIR}/dirty-chaos-code-tree/allowed.log"

grep -q 'chaos code_tree_state: dirty (risk-accepted)' "${TMP_DIR}/dirty-chaos-code-tree/allowed.log" \
  || fail "expected accepted dirty chaos code tree to be reported"

mkdir -p "${TMP_DIR}/missing-chaos-code-tree"
write_conformance_metadata \
  "${TMP_DIR}/missing-chaos-code-tree/conformance.yaml" \
  "${candidate_short}"
write_benchmark_metadata \
  "${TMP_DIR}/missing-chaos-code-tree/performance.txt" \
  "${candidate_full}" \
  "clean"
cat >"${TMP_DIR}/missing-chaos-code-tree/chaos.txt" <<EOF
captured_at=2026-04-24T08:00:00+08:00
git_commit=${candidate_full}
git_tree_state=dirty
run_id=test-run
EOF
write_chaos_conclusions_summary "${TMP_DIR}/missing-chaos-code-tree/chaos.txt"
write_soak_metadata \
  "${TMP_DIR}/missing-chaos-code-tree/soak.txt" \
  "${candidate_full}" \
  "clean"

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/missing-chaos-code-tree/conformance.yaml" \
  --performance "${TMP_DIR}/missing-chaos-code-tree/performance.txt" \
  --chaos "${TMP_DIR}/missing-chaos-code-tree/chaos.txt" \
  --soak "${TMP_DIR}/missing-chaos-code-tree/soak.txt" \
  >"${TMP_DIR}/missing-chaos-code-tree/stdout.log" 2>"${TMP_DIR}/missing-chaos-code-tree/stderr.log"; then
  fail "expected missing chaos code_tree_state to be rejected"
fi

grep -q 'chaos evidence is missing code_tree_state' "${TMP_DIR}/missing-chaos-code-tree/stderr.log" \
  || fail "expected missing chaos code_tree_state rejection reason"

"${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --allow-dirty-chaos-code-tree \
  --conformance "${TMP_DIR}/missing-chaos-code-tree/conformance.yaml" \
  --performance "${TMP_DIR}/missing-chaos-code-tree/performance.txt" \
  --chaos "${TMP_DIR}/missing-chaos-code-tree/chaos.txt" \
  --soak "${TMP_DIR}/missing-chaos-code-tree/soak.txt" \
  >"${TMP_DIR}/missing-chaos-code-tree/allowed.log"

grep -q 'chaos code_tree_state: missing (risk-accepted)' "${TMP_DIR}/missing-chaos-code-tree/allowed.log" \
  || fail "expected accepted missing chaos code tree to be reported"

mkdir -p "${TMP_DIR}/dirty-soak-code-tree"
write_conformance_metadata \
  "${TMP_DIR}/dirty-soak-code-tree/conformance.yaml" \
  "${candidate_short}"
write_benchmark_metadata \
  "${TMP_DIR}/dirty-soak-code-tree/performance.txt" \
  "${candidate_full}" \
  "clean"
write_benchmark_metadata \
  "${TMP_DIR}/dirty-soak-code-tree/chaos.txt" \
  "${candidate_full}" \
  "clean"
write_chaos_conclusions_summary "${TMP_DIR}/dirty-soak-code-tree/chaos.txt"
write_soak_metadata \
  "${TMP_DIR}/dirty-soak-code-tree/soak.txt" \
  "${candidate_full}" \
  "dirty"

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/dirty-soak-code-tree/conformance.yaml" \
  --performance "${TMP_DIR}/dirty-soak-code-tree/performance.txt" \
  --chaos "${TMP_DIR}/dirty-soak-code-tree/chaos.txt" \
  --soak "${TMP_DIR}/dirty-soak-code-tree/soak.txt" \
  >"${TMP_DIR}/dirty-soak-code-tree/stdout.log" 2>"${TMP_DIR}/dirty-soak-code-tree/stderr.log"; then
  fail "expected dirty soak code tree to be rejected"
fi

grep -q 'soak evidence code_tree_state dirty' "${TMP_DIR}/dirty-soak-code-tree/stderr.log" \
  || fail "expected dirty soak code tree rejection reason"

"${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --allow-dirty-soak-code-tree \
  --conformance "${TMP_DIR}/dirty-soak-code-tree/conformance.yaml" \
  --performance "${TMP_DIR}/dirty-soak-code-tree/performance.txt" \
  --chaos "${TMP_DIR}/dirty-soak-code-tree/chaos.txt" \
  --soak "${TMP_DIR}/dirty-soak-code-tree/soak.txt" \
  >"${TMP_DIR}/dirty-soak-code-tree/allowed.log"

grep -q 'soak code_tree_state: dirty (risk-accepted)' "${TMP_DIR}/dirty-soak-code-tree/allowed.log" \
  || fail "expected accepted dirty soak code tree to be reported"

mkdir -p "${TMP_DIR}/missing-soak-code-tree"
write_conformance_metadata \
  "${TMP_DIR}/missing-soak-code-tree/conformance.yaml" \
  "${candidate_short}"
write_benchmark_metadata \
  "${TMP_DIR}/missing-soak-code-tree/performance.txt" \
  "${candidate_full}" \
  "clean"
write_benchmark_metadata \
  "${TMP_DIR}/missing-soak-code-tree/chaos.txt" \
  "${candidate_full}" \
  "clean"
write_chaos_conclusions_summary "${TMP_DIR}/missing-soak-code-tree/chaos.txt"
cat >"${TMP_DIR}/missing-soak-code-tree/soak.txt" <<EOF
captured_at=2026-04-24T08:00:00+08:00
git_commit=${candidate_full}
git_tree_state=dirty
run_id=test-run
duration_seconds=86400
EOF

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/missing-soak-code-tree/conformance.yaml" \
  --performance "${TMP_DIR}/missing-soak-code-tree/performance.txt" \
  --chaos "${TMP_DIR}/missing-soak-code-tree/chaos.txt" \
  --soak "${TMP_DIR}/missing-soak-code-tree/soak.txt" \
  >"${TMP_DIR}/missing-soak-code-tree/stdout.log" 2>"${TMP_DIR}/missing-soak-code-tree/stderr.log"; then
  fail "expected missing soak code_tree_state to be rejected"
fi

grep -q 'soak evidence is missing code_tree_state' "${TMP_DIR}/missing-soak-code-tree/stderr.log" \
  || fail "expected missing soak code_tree_state rejection reason"

"${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --allow-dirty-soak-code-tree \
  --conformance "${TMP_DIR}/missing-soak-code-tree/conformance.yaml" \
  --performance "${TMP_DIR}/missing-soak-code-tree/performance.txt" \
  --chaos "${TMP_DIR}/missing-soak-code-tree/chaos.txt" \
  --soak "${TMP_DIR}/missing-soak-code-tree/soak.txt" \
  >"${TMP_DIR}/missing-soak-code-tree/allowed.log"

grep -q 'soak code_tree_state: missing (risk-accepted)' "${TMP_DIR}/missing-soak-code-tree/allowed.log" \
  || fail "expected accepted missing soak code tree to be reported"

mkdir -p "${TMP_DIR}/short-soak"
write_conformance_metadata \
  "${TMP_DIR}/short-soak/conformance.yaml" \
  "${candidate_short}"
write_benchmark_metadata \
  "${TMP_DIR}/short-soak/performance.txt" \
  "${candidate_full}" \
  "clean"
write_benchmark_metadata \
  "${TMP_DIR}/short-soak/chaos.txt" \
  "${candidate_full}" \
  "clean"
write_chaos_conclusions_summary "${TMP_DIR}/short-soak/chaos.txt"
write_soak_metadata \
  "${TMP_DIR}/short-soak/soak.txt" \
  "${candidate_full}" \
  "clean" \
  "600"

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/short-soak/conformance.yaml" \
  --performance "${TMP_DIR}/short-soak/performance.txt" \
  --chaos "${TMP_DIR}/short-soak/chaos.txt" \
  --soak "${TMP_DIR}/short-soak/soak.txt" \
  >"${TMP_DIR}/short-soak/stdout.log" 2>"${TMP_DIR}/short-soak/stderr.log"; then
  fail "expected short soak evidence to be rejected"
fi

grep -q 'soak evidence duration_seconds 600 is less than required 86400' "${TMP_DIR}/short-soak/stderr.log" \
  || fail "expected short soak rejection reason"

mkdir -p \
  "${TMP_DIR}/failed-soak-traffic-slo/chaos" \
  "${TMP_DIR}/failed-soak-traffic-slo/soak"
write_conformance_metadata \
  "${TMP_DIR}/failed-soak-traffic-slo/conformance.yaml" \
  "${candidate_short}"
write_benchmark_metadata \
  "${TMP_DIR}/failed-soak-traffic-slo/performance.txt" \
  "${candidate_full}" \
  "clean"
write_benchmark_metadata \
  "${TMP_DIR}/failed-soak-traffic-slo/chaos/metadata.txt" \
  "${candidate_full}" \
  "clean"
write_chaos_conclusions_summary "${TMP_DIR}/failed-soak-traffic-slo/chaos/metadata.txt"
write_soak_metadata \
  "${TMP_DIR}/failed-soak-traffic-slo/soak/metadata.txt" \
  "${candidate_full}" \
  "clean"
write_soak_traffic_summary "${TMP_DIR}/failed-soak-traffic-slo/soak/metadata.txt" "fail"

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/failed-soak-traffic-slo/conformance.yaml" \
  --performance "${TMP_DIR}/failed-soak-traffic-slo/performance.txt" \
  --chaos "${TMP_DIR}/failed-soak-traffic-slo/chaos/metadata.txt" \
  --soak "${TMP_DIR}/failed-soak-traffic-slo/soak/metadata.txt" \
  >"${TMP_DIR}/failed-soak-traffic-slo/stdout.log" 2>"${TMP_DIR}/failed-soak-traffic-slo/stderr.log"; then
  fail "expected failed soak traffic SLO gate to be rejected"
fi

grep -q 'soak evidence traffic SLO gate fail is not pass' "${TMP_DIR}/failed-soak-traffic-slo/stderr.log" \
  || fail "expected failed soak traffic SLO rejection reason"

mkdir -p \
  "${TMP_DIR}/missing-soak-traffic-slo/chaos" \
  "${TMP_DIR}/missing-soak-traffic-slo/soak"
write_conformance_metadata \
  "${TMP_DIR}/missing-soak-traffic-slo/conformance.yaml" \
  "${candidate_short}"
write_benchmark_metadata \
  "${TMP_DIR}/missing-soak-traffic-slo/performance.txt" \
  "${candidate_full}" \
  "clean"
write_benchmark_metadata \
  "${TMP_DIR}/missing-soak-traffic-slo/chaos/metadata.txt" \
  "${candidate_full}" \
  "clean"
write_chaos_conclusions_summary "${TMP_DIR}/missing-soak-traffic-slo/chaos/metadata.txt"
write_soak_metadata \
  "${TMP_DIR}/missing-soak-traffic-slo/soak/metadata.txt" \
  "${candidate_full}" \
  "clean"
rm -rf "${TMP_DIR}/missing-soak-traffic-slo/soak/traffic"

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/missing-soak-traffic-slo/conformance.yaml" \
  --performance "${TMP_DIR}/missing-soak-traffic-slo/performance.txt" \
  --chaos "${TMP_DIR}/missing-soak-traffic-slo/chaos/metadata.txt" \
  --soak "${TMP_DIR}/missing-soak-traffic-slo/soak/metadata.txt" \
  >"${TMP_DIR}/missing-soak-traffic-slo/stdout.log" 2>"${TMP_DIR}/missing-soak-traffic-slo/stderr.log"; then
  fail "expected missing soak traffic SLO summary to be rejected"
fi

grep -q 'soak evidence is missing traffic SLO summary' "${TMP_DIR}/missing-soak-traffic-slo/stderr.log" \
  || fail "expected missing soak traffic SLO rejection reason"

mkdir -p "${TMP_DIR}/incomplete-chaos"
write_conformance_metadata \
  "${TMP_DIR}/incomplete-chaos/conformance.yaml" \
  "${candidate_short}"
write_benchmark_metadata \
  "${TMP_DIR}/incomplete-chaos/performance.txt" \
  "${candidate_full}" \
  "clean"
write_benchmark_metadata \
  "${TMP_DIR}/incomplete-chaos/chaos.txt" \
  "${candidate_full}" \
  "clean"
write_chaos_conclusions_summary \
  "${TMP_DIR}/incomplete-chaos/chaos.txt" \
  "incomplete" \
  '["node-drain"]'
write_soak_metadata \
  "${TMP_DIR}/incomplete-chaos/soak.txt" \
  "${candidate_full}" \
  "clean"

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/incomplete-chaos/conformance.yaml" \
  --performance "${TMP_DIR}/incomplete-chaos/performance.txt" \
  --chaos "${TMP_DIR}/incomplete-chaos/chaos.txt" \
  --soak "${TMP_DIR}/incomplete-chaos/soak.txt" \
  >"${TMP_DIR}/incomplete-chaos/stdout.log" 2>"${TMP_DIR}/incomplete-chaos/stderr.log"; then
  fail "expected incomplete chaos evidence to be rejected"
fi

grep -q 'chaos evidence release_gate_status incomplete is not pass' "${TMP_DIR}/incomplete-chaos/stderr.log" \
  || fail "expected incomplete chaos rejection reason"

mkdir -p \
  "${TMP_DIR}/failed-chaos-traffic-slo/chaos" \
  "${TMP_DIR}/failed-chaos-traffic-slo/soak"
write_conformance_metadata \
  "${TMP_DIR}/failed-chaos-traffic-slo/conformance.yaml" \
  "${candidate_short}"
write_benchmark_metadata \
  "${TMP_DIR}/failed-chaos-traffic-slo/performance.txt" \
  "${candidate_full}" \
  "clean"
write_benchmark_metadata \
  "${TMP_DIR}/failed-chaos-traffic-slo/chaos/metadata.txt" \
  "${candidate_full}" \
  "clean"
write_chaos_conclusions_summary "${TMP_DIR}/failed-chaos-traffic-slo/chaos/metadata.txt"
write_chaos_traffic_summary "${TMP_DIR}/failed-chaos-traffic-slo/chaos/metadata.txt" "fail"
write_soak_metadata \
  "${TMP_DIR}/failed-chaos-traffic-slo/soak/metadata.txt" \
  "${candidate_full}" \
  "clean"

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/failed-chaos-traffic-slo/conformance.yaml" \
  --performance "${TMP_DIR}/failed-chaos-traffic-slo/performance.txt" \
  --chaos "${TMP_DIR}/failed-chaos-traffic-slo/chaos/metadata.txt" \
  --soak "${TMP_DIR}/failed-chaos-traffic-slo/soak/metadata.txt" \
  >"${TMP_DIR}/failed-chaos-traffic-slo/stdout.log" 2>"${TMP_DIR}/failed-chaos-traffic-slo/stderr.log"; then
  fail "expected failed chaos traffic SLO gate to be rejected"
fi

grep -q 'chaos evidence traffic SLO gate fail is not pass' "${TMP_DIR}/failed-chaos-traffic-slo/stderr.log" \
  || fail "expected failed chaos traffic SLO rejection reason"

mkdir -p \
  "${TMP_DIR}/missing-chaos-traffic-slo/chaos" \
  "${TMP_DIR}/missing-chaos-traffic-slo/soak"
write_conformance_metadata \
  "${TMP_DIR}/missing-chaos-traffic-slo/conformance.yaml" \
  "${candidate_short}"
write_benchmark_metadata \
  "${TMP_DIR}/missing-chaos-traffic-slo/performance.txt" \
  "${candidate_full}" \
  "clean"
write_benchmark_metadata \
  "${TMP_DIR}/missing-chaos-traffic-slo/chaos/metadata.txt" \
  "${candidate_full}" \
  "clean"
write_chaos_conclusions_summary "${TMP_DIR}/missing-chaos-traffic-slo/chaos/metadata.txt"
rm -rf "${TMP_DIR}/missing-chaos-traffic-slo/chaos/traffic"
write_soak_metadata \
  "${TMP_DIR}/missing-chaos-traffic-slo/soak/metadata.txt" \
  "${candidate_full}" \
  "clean"

if "${CHECK_SCRIPT}" \
  --candidate "${candidate_short}" \
  --conformance "${TMP_DIR}/missing-chaos-traffic-slo/conformance.yaml" \
  --performance "${TMP_DIR}/missing-chaos-traffic-slo/performance.txt" \
  --chaos "${TMP_DIR}/missing-chaos-traffic-slo/chaos/metadata.txt" \
  --soak "${TMP_DIR}/missing-chaos-traffic-slo/soak/metadata.txt" \
  >"${TMP_DIR}/missing-chaos-traffic-slo/stdout.log" 2>"${TMP_DIR}/missing-chaos-traffic-slo/stderr.log"; then
  fail "expected missing chaos traffic SLO summary to be rejected"
fi

grep -q 'chaos evidence is missing traffic SLO summary' "${TMP_DIR}/missing-chaos-traffic-slo/stderr.log" \
  || fail "expected missing chaos traffic SLO rejection reason"

printf '[release-evidence-test] ok\n'
