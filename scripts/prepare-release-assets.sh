#!/usr/bin/env bash
set -euo pipefail

tool_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
repo_root="${tool_root}"
release_evidence_candidate="${RELEASE_EVIDENCE_CANDIDATE:-}"
release_evidence_allow_commits="${RELEASE_EVIDENCE_ALLOW_COMMITS:-}"
release_install_profile="${RELEASE_INSTALL_PROFILE:-single-cluster-prod}"
release_controlplane_digest="${RELEASE_CONTROLPLANE_DIGEST:-}"
release_dataplane_digest="${RELEASE_DATAPLANE_DIGEST:-}"

usage() {
  printf 'usage: %s [--repo-root <path>] <release-tag> <controlplane-image> <dataplane-image> <output-dir>\n' "${0##*/}" >&2
  exit 1
}

fail() {
  printf 'prepare-release-assets: %s\n' "$*" >&2
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-root)
      [[ $# -ge 2 ]] || usage
      repo_root="$2"
      shift 2
      ;;
    *)
      break
      ;;
  esac
done

if [[ $# -ne 4 ]]; then
  usage
fi

RELEASE_TAG="$1"
CONTROL_IMAGE="$2"
DATAPLANE_IMAGE="$3"
OUTPUT_DIR="$4"

assert_file() {
  local path="$1"

  if [[ ! -f "${path}" ]]; then
    printf 'missing expected release asset: %s\n' "${path}" >&2
    exit 1
  fi
}

assert_contains() {
  local path="$1"
  local needle="$2"
  local description="$3"

  if ! grep -Fq -- "${needle}" "${path}"; then
    printf 'expected %s in %s\n' "${description}" "${path}" >&2
    exit 1
  fi
}

image_name_for_digest_reference() {
  local image_ref="$1"
  local without_digest
  local last_segment

  without_digest="${image_ref%@*}"
  last_segment="${without_digest##*/}"
  if [[ "${last_segment}" == *:* ]]; then
    printf '%s\n' "${without_digest%:*}"
    return
  fi

  printf '%s\n' "${without_digest}"
}

resolve_image_digest() {
  local label="$1"
  local image_ref="$2"
  local explicit_digest="$3"
  local digest=""

  if [[ -n "${explicit_digest}" ]]; then
    digest="${explicit_digest}"
  elif [[ "${image_ref}" == *@sha256:* ]]; then
    digest="${image_ref#*@}"
  else
    fail "missing ${label} image digest; set RELEASE_${label^^}_DIGEST or pass an image@sha256:<digest> reference"
  fi

  if [[ ! "${digest}" =~ ^sha256:[0-9a-fA-F]{64}$ ]]; then
    fail "invalid ${label} image digest: ${digest}"
  fi

  printf '%s\n' "${digest}"
}

write_image_digests() {
  local control_digest dataplane_digest
  local control_name dataplane_name

  control_digest="$(resolve_image_digest "controlplane" "${CONTROL_IMAGE}" "${release_controlplane_digest}")"
  dataplane_digest="$(resolve_image_digest "dataplane" "${DATAPLANE_IMAGE}" "${release_dataplane_digest}")"

  control_name="$(image_name_for_digest_reference "${CONTROL_IMAGE}")"
  dataplane_name="$(image_name_for_digest_reference "${DATAPLANE_IMAGE}")"

  cat >"${OUTPUT_DIR}/image-digests.txt" <<EOF
controlplane_image=${CONTROL_IMAGE}
controlplane_digest=${control_digest}
controlplane_reference=${control_name}@${control_digest}
dataplane_image=${DATAPLANE_IMAGE}
dataplane_digest=${dataplane_digest}
dataplane_reference=${dataplane_name}@${dataplane_digest}
EOF
}

write_release_notes() {
  cat >"${OUTPUT_DIR}/RELEASE_NOTES.md" <<EOF
# Aether Gateway ${RELEASE_TAG}

## Breaking Changes

- Review \`CHANGELOG.md\` and \`docs/user/compatibility-notes.md\` before upgrading. This generated release note does not replace the human-maintained compatibility summary.

## Upgrade Notes

- Install profile: \`${release_install_profile}\`.
- \`install.yaml\` is rendered from the selected install profile.
- For production deployments, create real Secrets from \`production/*.secret.example.yaml\` and replace local or placeholder status addresses before applying assets.
- Canary and rollback operations should follow \`docs/user/release-runbook.md\`.

## Security

- Image digest pins are recorded in \`image-digests.txt\`.
- Image SBOMs are expected as \`controlplane-image.spdx.json\` and \`dataplane-image.spdx.json\`.
- Image provenance and SBOM attestations are published by the release workflow through GitHub Artifact Attestations.
- The release asset bundle SBOM is expected as \`release-assets.spdx.json\`.

## Conformance

- Gateway API conformance evidence is expected as \`conformance-report.yaml\`, \`conformance-run.log\`, and \`conformance-metadata.yaml\` when the release workflow attaches conformance artifacts.
- The release workflow runs full Gateway API conformance with \`ALL_FEATURES=true\`.

## Performance

- Performance, chaos, and soak evidence references are tracked in \`CHANGELOG.md\`, \`docs/test/latest-baseline.md\`, and the \`reports/*/README.md\` files.
- Any accepted p99, success-rate, CPU, RSS, FD, or soak regression must be explained in the human-maintained changelog or release summary before publishing.

## Known Issues

- See \`CHANGELOG.md\`, \`docs/backlog/security.md\`, and \`docs/security/risk-register.md\` for current known issues, accepted risks, and dependency exceptions.

## Release Evidence Checklist

- Unit and guardrail tests: release workflow \`controlplane\` and \`dataplane\` jobs.
- Security scans: release workflow \`security-scans\` job.
- e2e / smoke: release workflow \`kind-smoke\` job.
- Conformance: release workflow full-suite conformance step.
- Image digests: \`image-digests.txt\`.
- SBOM / provenance: image and release-asset attestation steps in the release workflow.
EOF
}

run_release_evidence_gate() {
  local candidate="$1"
  local allow_args=()

  if [[ -n "${release_evidence_allow_commits}" ]]; then
    local allow_commit
    # Accept a simple whitespace-delimited window for shell and workflow env usage.
    read -r -a allow_commits <<<"${release_evidence_allow_commits}"
    for allow_commit in "${allow_commits[@]}"; do
      [[ -n "${allow_commit}" ]] || continue
      allow_args+=(--allow-commit "${allow_commit}")
    done
  fi

  "${tool_root}/scripts/refresh-release-evidence.sh" \
    --repo-root "${repo_root}" \
    --candidate "${candidate}" \
    "${allow_args[@]}" \
    --check-only
}

write_release_evidence_manifest() {
  local candidate="$1"

  {
    printf 'candidate=%s\n' "${candidate}"
    if [[ -n "${release_evidence_allow_commits}" ]]; then
      printf 'allowed_commits=%s\n' "${release_evidence_allow_commits}"
    else
      printf 'allowed_commits=\n'
    fi
    run_release_evidence_gate "${candidate}"
  } >"${OUTPUT_DIR}/release-evidence.txt"
}

verify_release_assets() {
  assert_file "${OUTPUT_DIR}/install.yaml"
  assert_file "${OUTPUT_DIR}/hpa.yaml"
  assert_file "${OUTPUT_DIR}/LICENSE"
  assert_file "${OUTPUT_DIR}/controlplane-config.example.yaml"
  assert_file "${OUTPUT_DIR}/dataplane-config.example.yaml"
  assert_file "${OUTPUT_DIR}/production/README.md"
  assert_file "${OUTPUT_DIR}/production/kustomization.yaml"
  assert_file "${OUTPUT_DIR}/production/controlplane-config.yaml"
  assert_file "${OUTPUT_DIR}/production/dataplane-config.yaml"
  assert_file "${OUTPUT_DIR}/production/controlplane-admin-auth.secret.example.yaml"
  assert_file "${OUTPUT_DIR}/production/controlplane-grpc-tls.secret.example.yaml"
  assert_file "${OUTPUT_DIR}/production/dataplane-admin-auth.secret.example.yaml"
  assert_file "${OUTPUT_DIR}/production/dataplane-session-persistence.secret.example.yaml"
  assert_file "${OUTPUT_DIR}/production/dataplane-xds-tls.secret.example.yaml"
  assert_file "${OUTPUT_DIR}/README.txt"
  assert_file "${OUTPUT_DIR}/RELEASE_NOTES.md"
  assert_file "${OUTPUT_DIR}/image-digests.txt"
  assert_file "${OUTPUT_DIR}/release-evidence.txt"
  assert_file "${OUTPUT_DIR}/checksums.txt"

  assert_contains "${OUTPUT_DIR}/install.yaml" "# Install profile: ${release_install_profile}" "release install profile"
  assert_contains "${OUTPUT_DIR}/install.yaml" "${CONTROL_IMAGE}" "controlplane image reference"
  assert_contains "${OUTPUT_DIR}/install.yaml" "${DATAPLANE_IMAGE}" "dataplane image reference"
  assert_contains "${OUTPUT_DIR}/README.txt" "${CONTROL_IMAGE}" "controlplane image reference"
  assert_contains "${OUTPUT_DIR}/README.txt" "${DATAPLANE_IMAGE}" "dataplane image reference"
  assert_contains "${OUTPUT_DIR}/README.txt" "Install profile: ${release_install_profile}" "release install profile"
  assert_contains "${OUTPUT_DIR}/README.txt" "Apache-2.0" "repository license reference"
  assert_contains "${OUTPUT_DIR}/README.txt" "install.yaml" "install manifest entry"
  assert_contains "${OUTPUT_DIR}/README.txt" "hpa.yaml" "hpa manifest entry"
  assert_contains "${OUTPUT_DIR}/README.txt" "production/" "production overlay entry"
  assert_contains "${OUTPUT_DIR}/RELEASE_NOTES.md" "## Breaking Changes" "release notes breaking changes section"
  assert_contains "${OUTPUT_DIR}/RELEASE_NOTES.md" "## Upgrade Notes" "release notes upgrade section"
  assert_contains "${OUTPUT_DIR}/RELEASE_NOTES.md" "## Security" "release notes security section"
  assert_contains "${OUTPUT_DIR}/RELEASE_NOTES.md" "## Conformance" "release notes conformance section"
  assert_contains "${OUTPUT_DIR}/RELEASE_NOTES.md" "## Performance" "release notes performance section"
  assert_contains "${OUTPUT_DIR}/RELEASE_NOTES.md" "## Known Issues" "release notes known issues section"
  assert_contains "${OUTPUT_DIR}/image-digests.txt" "controlplane_digest=" "controlplane digest entry"
  assert_contains "${OUTPUT_DIR}/image-digests.txt" "dataplane_digest=" "dataplane digest entry"
  assert_contains "${OUTPUT_DIR}/release-evidence.txt" "candidate=" "release evidence candidate entry"
  assert_contains "${OUTPUT_DIR}/release-evidence.txt" "selected conformance metadata:" "release evidence conformance entry"
  assert_contains "${OUTPUT_DIR}/release-evidence.txt" "selected performance metadata:" "release evidence performance entry"
  assert_contains "${OUTPUT_DIR}/release-evidence.txt" "selected chaos metadata:" "release evidence chaos entry"
  assert_contains "${OUTPUT_DIR}/release-evidence.txt" "selected soak metadata:" "release evidence soak entry"
  assert_contains "${OUTPUT_DIR}/checksums.txt" "  ./LICENSE" "license checksum entry"
  assert_contains "${OUTPUT_DIR}/checksums.txt" "  ./RELEASE_NOTES.md" "release notes checksum entry"
  assert_contains "${OUTPUT_DIR}/checksums.txt" "  ./README.txt" "README checksum entry"
  assert_contains "${OUTPUT_DIR}/checksums.txt" "  ./controlplane-config.example.yaml" "controlplane config checksum entry"
  assert_contains "${OUTPUT_DIR}/checksums.txt" "  ./dataplane-config.example.yaml" "dataplane config checksum entry"
  assert_contains "${OUTPUT_DIR}/checksums.txt" "  ./hpa.yaml" "hpa checksum entry"
  assert_contains "${OUTPUT_DIR}/checksums.txt" "  ./image-digests.txt" "image digest checksum entry"
  assert_contains "${OUTPUT_DIR}/checksums.txt" "  ./install.yaml" "install manifest checksum entry"
  assert_contains "${OUTPUT_DIR}/checksums.txt" "  ./release-evidence.txt" "release evidence checksum entry"
  assert_contains "${OUTPUT_DIR}/checksums.txt" "  ./production/README.md" "production README checksum entry"
  assert_contains "${OUTPUT_DIR}/checksums.txt" "  ./production/kustomization.yaml" "production kustomization checksum entry"
}

mkdir -p "${OUTPUT_DIR}"

if [[ -z "${release_evidence_candidate}" ]]; then
  release_evidence_candidate="$(git -C "${repo_root}" rev-parse --short HEAD)"
fi

write_release_evidence_manifest "${release_evidence_candidate}"
"${tool_root}/scripts/check-evidence-reference-alignment.sh" --repo-root "${repo_root}"

"${tool_root}/scripts/render-release-manifest.sh" \
  --repo-root "${repo_root}" \
  --profile "${release_install_profile}" \
  "${CONTROL_IMAGE}" \
  "${DATAPLANE_IMAGE}" \
  "${OUTPUT_DIR}/install.yaml"

cp "${repo_root}/LICENSE" "${OUTPUT_DIR}/LICENSE"
cp "${repo_root}/deploy/kubernetes/addons/dataplane-hpa/hpa.yaml" "${OUTPUT_DIR}/hpa.yaml"
cp "${repo_root}/configs/controlplane/config.yaml" "${OUTPUT_DIR}/controlplane-config.example.yaml"
cp "${repo_root}/configs/dataplane/config.yaml" "${OUTPUT_DIR}/dataplane-config.example.yaml"
mkdir -p "${OUTPUT_DIR}/production"
cp -R "${repo_root}/deploy/kubernetes/overlays/production/." "${OUTPUT_DIR}/production/"

cat >"${OUTPUT_DIR}/README.txt" <<EOF
Aether Gateway ${RELEASE_TAG}

Included assets:
- LICENSE
- install.yaml
- image-digests.txt
- release-evidence.txt
- RELEASE_NOTES.md
- hpa.yaml
- controlplane-config.example.yaml
- dataplane-config.example.yaml
- production/

Published images:
- ${CONTROL_IMAGE}
- ${DATAPLANE_IMAGE}

Notes:
- Install profile: ${release_install_profile}
- install.yaml is rendered from the install profile selected by RELEASE_INSTALL_PROFILE, defaulting to single-cluster-prod.
- production/ contains the hardened Kustomize overlay, config files and example Secrets for long-lived environments.
- For production deployment, create real Secrets from production/*.secret.example.yaml, replace placeholder addresses, then apply install.yaml or kubectl apply -k production/.
- hpa.yaml is also included as a standalone reference; production install profiles already render the dataplane HPA.
- The source repository and release assets are licensed under Apache-2.0; see LICENSE.
- release automation may attach conformance-report.yaml, conformance-run.log, conformance-metadata.yaml, image-digests.txt, *-image.spdx.json, and release-assets.spdx.json.
- Long-lived environments must not keep local or placeholder statusAddress values.
EOF

write_image_digests
write_release_notes

(
  cd "${OUTPUT_DIR}"
  find . -type f ! -name checksums.txt -print0 \
    | sort -z \
    | xargs -0 sha256sum >checksums.txt
)

verify_release_assets
