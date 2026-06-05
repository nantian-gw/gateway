#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PLAN_ONLY="${PLAN_ONLY:-false}"
INCLUDE_KIND="${INCLUDE_KIND:-false}"
SKIP_BUILD="${SKIP_BUILD:-true}"
BASE_REF="${BASE_REF:-HEAD}"

declare -a FILES=()
declare -a SHELL_SCRIPTS=()
declare -a NOTES=()
declare -a COMMANDS=()
declare -a REASONS=()
declare -a SKIPPED_VALIDATIONS=()
declare -a SKIPPED_REASONS=()
declare -a SKIPPED_ENABLES=()

NEEDS_PROTO=false
NEEDS_CONTROLPLANE=false
NEEDS_DATAPLANE=false
NEEDS_KIND=false
NEEDS_METRICS_SURFACES=false
NEEDS_METRICS_CARDINALITY_CONTRACT=false
NEEDS_COMMUNITY_GOVERNANCE_CONTRACT=false
NEEDS_SCRIPT_INVENTORY_CONTRACT=false
NEEDS_DEPENDABOT_ALERT_TRIAGE_CONTRACT=false
SEEN_NON_DOC_CHANGE=false

log() {
  printf '[targeted-validation] %s\n' "$*"
}

add_note() {
  local note="$1"
  local existing
  for existing in "${NOTES[@]:-}"; do
    if [[ "${existing}" == "${note}" ]]; then
      return
    fi
  done
  NOTES+=("${note}")
}

add_shell_script() {
  local path="$1"
  local existing
  for existing in "${SHELL_SCRIPTS[@]:-}"; do
    if [[ "${existing}" == "${path}" ]]; then
      return
    fi
  done
  SHELL_SCRIPTS+=("${path}")
}

add_command() {
  local command="$1"
  local reason="${2:-}"
  local existing
  local index
  for index in "${!COMMANDS[@]}"; do
    existing="${COMMANDS[${index}]}"
    if [[ "${existing}" == "${command}" ]]; then
      if [[ -n "${reason}" && -z "${REASONS[${index}]:-}" ]]; then
        REASONS[${index}]="${reason}"
      fi
      return
    fi
  done
  COMMANDS+=("${command}")
  REASONS+=("${reason}")
}

add_skipped_validation() {
  local command="$1"
  local reason="$2"
  local enable="${3:-}"
  local existing
  local index
  for index in "${!SKIPPED_VALIDATIONS[@]}"; do
    existing="${SKIPPED_VALIDATIONS[${index}]}"
    if [[ "${existing}" == "${command}" ]]; then
      return
    fi
  done
  SKIPPED_VALIDATIONS+=("${command}")
  SKIPPED_REASONS+=("${reason}")
  SKIPPED_ENABLES+=("${enable}")
}

join_by() {
  local separator="$1"
  shift
  local first=true
  local item
  for item in "$@"; do
    if [[ "${first}" == true ]]; then
      printf '%s' "${item}"
      first=false
      continue
    fi
    printf '%s%s' "${separator}" "${item}"
  done
}

normalize_path() {
  local raw="$1"
  raw="${raw#./}"
  if [[ "${raw}" == "${ROOT_DIR}"/* ]]; then
    raw="${raw#"${ROOT_DIR}/"}"
  fi
  printf '%s\n' "${raw}"
}

collect_git_files() {
  local tracked_output untracked_output path

  tracked_output="$(git -C "${ROOT_DIR}" diff --name-only --relative "${BASE_REF}" 2>/dev/null || true)"
  untracked_output="$(git -C "${ROOT_DIR}" ls-files --others --exclude-standard 2>/dev/null || true)"

  while IFS= read -r path; do
    [[ -n "${path}" ]] || continue
    FILES+=("${path}")
  done <<<"${tracked_output}"

  while IFS= read -r path; do
    [[ -n "${path}" ]] || continue
    FILES+=("${path}")
  done <<<"${untracked_output}"
}

collect_input_files() {
  local path
  if [[ "$#" -gt 0 ]]; then
    for path in "$@"; do
      FILES+=("$(normalize_path "${path}")")
    done
    return
  fi
  collect_git_files
}

classify_file() {
  local path="$1"

  case "${path}" in
    proto/*)
      NEEDS_PROTO=true
      NEEDS_CONTROLPLANE=true
      NEEDS_DATAPLANE=true
      ;;
    controlplane/*)
      NEEDS_CONTROLPLANE=true
      ;;
    dataplane/*)
      NEEDS_DATAPLANE=true
      ;;
  esac

  case "${path}" in
    *.sh)
      SEEN_NON_DOC_CHANGE=true
      if [[ -f "${ROOT_DIR}/${path}" ]]; then
        add_shell_script "${path}"
      fi
      ;;
  esac

  case "${path}" in
    scripts/*.sh|\
    scripts/lib/*.sh|\
    scripts/script-inventory.yaml|\
    docs/developer/scripts.md|\
    .agents/skills/aether-repo-scripts/SKILL.md|\
    .agents/skills/aether-repo-scripts/references/script-catalog.md|\
    tests/scripts/script-inventory.sh)
      NEEDS_SCRIPT_INVENTORY_CONTRACT=true
      ;;
  esac

  case "${path}" in
    scripts/check-dependabot-alert-triage.sh|\
    scripts/run-security-scans.sh|\
    tests/scripts/dependabot-alert-triage.sh|\
    tests/scripts/run-security-scans.sh|\
    docs/backlog/security.md|\
    docs/security/risk-register.md|\
    dataplane/deny.toml)
      NEEDS_DEPENDABOT_ALERT_TRIAGE_CONTRACT=true
      ;;
  esac

  case "${path}" in
    scripts/collect-admin-snapshots.sh|\
    tests/e2e/validate-metrics-surfaces.sh|\
    controlplane/cmd/manager/app.go|\
    controlplane/internal/observability/*|\
    controlplane/internal/admin/metrics.go|\
    controlplane/internal/admin/server*.go|\
    dataplane/crates/aeg-app/src/admin.rs|\
    dataplane/crates/aeg-app/src/admin/*|\
    dataplane/crates/aeg-observability/*|\
    dataplane/crates/aeg-xds/src/stats.rs)
      NEEDS_METRICS_SURFACES=true
      NEEDS_METRICS_CARDINALITY_CONTRACT=true
      ;;
  esac

  case "${path}" in
    docs/contracts/metrics-cardinality.md|\
    scripts/check-metrics-cardinality-contract.sh|\
    deploy/observability/grafana/*|\
    deploy/observability/prometheus/*)
      NEEDS_METRICS_CARDINALITY_CONTRACT=true
      ;;
  esac

  case "${path}" in
    README.md|\
    CONTRIBUTING.md|\
    CODE_OF_CONDUCT.md|\
    SECURITY.md|\
    SUPPORT.md|\
    VERSIONING.md|\
    ROADMAP.md|\
    GOVERNANCE.md|\
    MAINTAINERS.md|\
    docs/community-readiness.md|\
    docs/adopters-and-compatibility.md|\
    scripts/check-community-governance-contract.sh)
      NEEDS_COMMUNITY_GOVERNANCE_CONTRACT=true
      ;;
  esac

  case "${path}" in
    deploy/*|tests/e2e/*|tests/conformance/*)
      SEEN_NON_DOC_CHANGE=true
      NEEDS_KIND=true
      ;;
    configs/*)
      SEEN_NON_DOC_CHANGE=true
      add_note "Config changes detected; consider local process validation if behavior changed."
      ;;
    docs/*|*.md)
      ;;
    *)
      SEEN_NON_DOC_CHANGE=true
      ;;
  esac
}

build_plan() {
  local path
  local shell_reason

  for path in "${FILES[@]:-}"; do
    [[ -n "${path}" ]] || continue
    classify_file "${path}"
  done

  if [[ "${#SHELL_SCRIPTS[@]}" -gt 0 ]]; then
    shell_reason="shell scripts changed: $(join_by ', ' "${SHELL_SCRIPTS[@]}")"
    add_command "bash -n ${SHELL_SCRIPTS[*]}" "${shell_reason}"
  fi
  if [[ "${NEEDS_PROTO}" == true ]]; then
    add_command "make proto" "proto or shared contract changed"
  fi
  if [[ "${NEEDS_CONTROLPLANE}" == true ]]; then
    add_command "cd controlplane && go test ./..." "controlplane behavior or generated Go bindings may have changed"
  fi
  if [[ "${NEEDS_DATAPLANE}" == true ]]; then
    add_command "cargo test --manifest-path dataplane/Cargo.toml --workspace" "dataplane behavior or generated Rust bindings may have changed"
  fi
  if [[ "${NEEDS_METRICS_CARDINALITY_CONTRACT}" == true ]]; then
    add_command "./scripts/check-metrics-cardinality-contract.sh" "metrics cardinality contract or observability assets changed"
  fi
  if [[ "${NEEDS_COMMUNITY_GOVERNANCE_CONTRACT}" == true ]]; then
    add_command "./scripts/check-community-governance-contract.sh" "community governance or public project contract changed"
  fi
  if [[ "${NEEDS_SCRIPT_INVENTORY_CONTRACT}" == true ]]; then
    add_command "./scripts/check-script-inventory.sh" "script inventory, helper, or script docs changed"
  fi
  if [[ "${NEEDS_DEPENDABOT_ALERT_TRIAGE_CONTRACT}" == true ]]; then
    add_command "bash tests/scripts/dependabot-alert-triage.sh" "Dependabot alert triage policy changed"
    add_command "bash tests/scripts/run-security-scans.sh" "security scan bundle or Dependabot triage gate changed"
  fi
  if [[ "${NEEDS_KIND}" == true || "${NEEDS_METRICS_SURFACES}" == true ]]; then
    if [[ "${INCLUDE_KIND}" == true ]]; then
      add_command "SKIP_BUILD=${SKIP_BUILD} ./tests/e2e/run-kind.sh" "cluster-facing changes need Kind smoke validation and INCLUDE_KIND=true"
      if [[ "${NEEDS_METRICS_SURFACES}" == true ]]; then
        add_command "./tests/e2e/validate-metrics-surfaces.sh" "metrics/admin surface changes need live kind metrics consistency validation"
      fi
    else
      if [[ "${NEEDS_KIND}" == true ]]; then
        add_skipped_validation \
          "SKIP_BUILD=${SKIP_BUILD} ./tests/e2e/run-kind.sh" \
          "cluster-facing validation is disabled by default for targeted validation and INCLUDE_KIND=false" \
          "rerun with INCLUDE_KIND=true"
        add_note "Kind-affecting changes detected; rerun with INCLUDE_KIND=true to add smoke validation."
      fi
      if [[ "${NEEDS_METRICS_SURFACES}" == true ]]; then
        add_skipped_validation \
          "./tests/e2e/validate-metrics-surfaces.sh" \
          "metrics/admin surface validation requires live Kind services and INCLUDE_KIND=false" \
          "rerun with INCLUDE_KIND=true"
        add_note "Metrics/admin surface changes detected; INCLUDE_KIND=true will also run ./tests/e2e/validate-metrics-surfaces.sh."
      fi
    fi
  fi

  if [[ "${SEEN_NON_DOC_CHANGE}" == false ]]; then
    add_note "Documentation-only changes do not require runtime validation."
  fi
}

print_plan() {
  local path command note index reason

  if [[ "${#FILES[@]}" -eq 0 ]]; then
    log "no changed files detected"
    return
  fi

  log "changed files:"
  for path in "${FILES[@]}"; do
    printf '  - %s\n' "${path}"
  done

  if [[ "${#COMMANDS[@]}" -eq 0 ]]; then
    log "no validation commands selected"
  else
    log "validation plan:"
    for index in "${!COMMANDS[@]}"; do
      command="${COMMANDS[${index}]}"
      reason="${REASONS[${index}]:-}"
      if [[ -n "${reason}" ]]; then
        printf '  - %s\n    reason: %s\n' "${command}" "${reason}"
      else
        printf '  - %s\n' "${command}"
      fi
    done
  fi

  if [[ "${#NOTES[@]}" -gt 0 ]]; then
    log "notes:"
    for note in "${NOTES[@]}"; do
      printf '  - %s\n' "${note}"
    done
  fi

  if [[ "${#SKIPPED_VALIDATIONS[@]}" -gt 0 ]]; then
    log "skipped validations:"
    for index in "${!SKIPPED_VALIDATIONS[@]}"; do
      printf '  - %s\n' "${SKIPPED_VALIDATIONS[${index}]}"
      printf '    reason: %s\n' "${SKIPPED_REASONS[${index}]}"
      if [[ -n "${SKIPPED_ENABLES[${index}]:-}" ]]; then
        printf '    enable: %s\n' "${SKIPPED_ENABLES[${index}]}"
      fi
    done
  fi
}

run_plan() {
  local command

  if [[ "${#COMMANDS[@]}" -eq 0 ]]; then
    return
  fi

  for command in "${COMMANDS[@]}"; do
    log "running: ${command}"
    (
      cd "${ROOT_DIR}"
      eval "${command}"
    )
  done
}

main() {
  collect_input_files "$@"
  build_plan
  print_plan
  if [[ "${PLAN_ONLY}" == "true" ]]; then
    log "plan only; skipping execution"
    return
  fi
  run_plan
}

main "$@"
