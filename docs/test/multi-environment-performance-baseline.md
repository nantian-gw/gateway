# Multi-Environment Performance Baseline

This document defines the evidence required to close the multi-environment
performance and capacity baseline gate.

It complements `docs/test/performance-baseline.md`, which explains how to run a
single baseline. This document explains when the project has enough comparable
evidence across environments to make stronger capacity claims.

## Current Status

Status as of 2026-05-13: not satisfied.

The repository has repeatable Kind, local benchmark, and throughput-report
artifacts. That is useful regression evidence, but it is not enough to claim a
production-like or multi-environment capacity baseline. The multi-environment
performance TODO item must stay open until at least one non-Kind
production-like environment is archived using the same reporting contract and
compared against the existing local baseline.

## Environment Classes

| Class | Counts toward the multi-environment gate | Purpose |
| --- | --- | --- |
| `local-kind` | no by itself | Cheap repeatable smoke and regression baseline. |
| `local-host-bench` | no by itself | Microbenchmarks for reload, routing, allocation, parsing, and hot paths. |
| `multi-node-kind` | partial | Useful for topology and scheduling variance, but still not production-like enough by itself. |
| `non-kind-lab` | yes | Self-managed Kubernetes, bare metal, or VM lab with real node networking. |
| `managed-kubernetes` | yes | GKE, EKS, AKS, OpenShift, or equivalent managed cluster. |
| `production-canary` | yes | Controlled real workload or pre-production environment with production-like traffic and operations. |

The minimum closing condition is:

- one current local baseline, usually Kind A4 or an equivalent repo-standard run;
- one non-Kind `non-kind-lab`, `managed-kubernetes`, or `production-canary`
  baseline for the same commit or accepted release candidate; and
- one comparison report showing latency, success rate, throughput, and resource
  deltas.

## Required Metadata

Every run that counts toward the multi-environment gate must include a
`metadata.txt` or equivalent machine-readable metadata file with these fields:

```text
git_commit=<short-or-full-sha>
code_tree_state=clean
environment_class=<local-kind|multi-node-kind|non-kind-lab|managed-kubernetes|production-canary>
kubernetes_distribution=<kind|kubeadm|gke|eks|aks|openshift|other>
kubernetes_version=<version>
gateway_api_version=<version>
node_count=<integer>
node_shape=<cpu-memory-or-instance-type>
cni=<name-and-version>
load_balancer_mode=<nodeport|loadbalancer|hostnetwork|other>
dataplane_replicas=<integer>
controlplane_replicas=<integer>
traffic_profile=<profile-name>
```

If an environment cannot publish a detail for security reasons, record
`redacted` and explain why in `summary.md`.

## Required Metrics

Each comparable run must report:

- request or RPC success rate;
- request count and error count;
- p95, p99, and max latency for HTTP and gRPC when applicable;
- TCP and UDP packet/request loss where those routes are in scope;
- throughput or RPS for each profile;
- dataplane RSS, FD, thread count, and CPU snapshot;
- xDS reconnect count and NACK delta;
- snapshot ACK latency p99 or equivalent control-plane convergence signal;
- ready replica min/max/last;
- test duration and traffic generator shape.

## Recommended Collection Flow

For the local baseline, prefer the existing repo entrypoint:

```bash
./scripts/run-kind-a4-baseline.sh
```

For non-Kind environments, reuse the same artifact layout under
`reports/performance/runs/<run-id>/`:

```text
reports/performance/runs/<run-id>/
├── metadata.txt
├── summary.md
├── slo-gate.json
├── admin-before/
├── admin-after/
├── http/
├── grpc/
└── metrics.prom
```

The exact traffic generator may differ by environment, but the summary must
normalize the same fields so `scripts/compare-performance-runs.sh` can compare
the local and non-Kind run.

## Comparison Requirement

After collecting both runs, compare them:

```bash
./scripts/compare-performance-runs.sh \
  reports/performance/runs/<local-baseline-run-id> \
  reports/performance/runs/<non-kind-run-id>
```

The resulting comparison under `reports/performance/comparisons/` must be linked
from `reports/performance/README.md` before the gate is considered satisfied.

## Acceptance Rules

The gate is satisfied only when:

- both compared runs are from the same commit or an explicitly accepted release
  candidate window;
- the non-Kind run has `code_tree_state=clean`;
- the comparison has `result=pass`, or every regression has an explicit release
  risk acceptance;
- the summary states which environment is the production-like comparator; and
- `docs/adopters-and-compatibility.md` and `docs/community-readiness.md` do not
  overstate the result as general production capacity.

## What Does Not Count

- A single Kind run.
- A single local microbenchmark run.
- An unarchived ad hoc performance note.
- A dirty-tree run, unless a release owner explicitly risk-accepts it for a
  non-release investigation.
- A non-Kind run with incomparable traffic shape and no normalization.

## Closing The Multi-Environment TODO

When the evidence exists, update:

- this document with the run IDs and comparison link;
- `reports/performance/README.md` with the multi-environment baseline summary;
- `docs/community-readiness.md` with a narrow statement of what was validated;
- `docs/adopters-and-compatibility.md` if the environment can be public; and
- the multi-environment performance TODO item to checked only after the above
  evidence is public and reviewable.
