.PHONY: build test benchmarks conformance e2e-smoke

CLUSTER_NAME ?= nantian-conformance
CONFORMANCE_TIMEOUT ?= 300
CONTROL_PLANE_IMAGE ?= ghcr.io/nantian-gw/nantian-controlplane:latest
DATA_PLANE_IMAGE ?= ghcr.io/nantian-gw/dataplane:latest
DASHBOARD_IMAGE ?= ghcr.io/nantian-gw/dashboard:latest
CONFORMANCE_ECHO_BASIC_IMAGE ?= gcr.io/k8s-staging-gateway-api/echo-basic:v20260204-monthly-2026.01-60-g28382302
CONFORMANCE_COREDNS_IMAGE ?= registry.k8s.io/coredns/coredns:v1.12.2
CONFORMANCE_ECHO_ADVANCED_IMAGE ?= gcr.io/k8s-staging-gateway-api/echo-advanced:v20240412-v1.0.0-394-g40c666fd
CONFORMANCE_TEST_IMAGES ?= $(CONFORMANCE_ECHO_BASIC_IMAGE) $(CONFORMANCE_COREDNS_IMAGE) $(CONFORMANCE_ECHO_ADVANCED_IMAGE)
GATEWAY_API_CRDS_STANDARD ?= https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.5.1/standard-install.yaml
GATEWAY_API_CRDS_EXPERIMENTAL ?= https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.5.1/experimental-install.yaml

build:
	go build ./...

test:
	go test -count=1 -timeout 5m ./...

benchmarks:
	go test -bench=. -benchtime=1x -count=1 ./...

conformance:
	@echo "=== Creating kind cluster: $(CLUSTER_NAME) ==="
	CLUSTER_NAME=$(CLUSTER_NAME) scripts/ci/create-kind-cluster.sh
	@echo "=== Installing Gateway API CRDs ==="
	GATEWAY_API_CHANNEL=experimental scripts/ci/install-gateway-api-crds.sh
	@echo "=== Preloading nantian-gw images ==="
	CONFORMANCE_TEST_IMAGES="$(CONFORMANCE_TEST_IMAGES)" CLUSTER_NAME=$(CLUSTER_NAME) CONTROL_PLANE_IMAGE=$(CONTROL_PLANE_IMAGE) DATA_PLANE_IMAGE=$(DATA_PLANE_IMAGE) DASHBOARD_IMAGE=$(DASHBOARD_IMAGE) scripts/ci/load-kind-images.sh
	@echo "=== Deploying nantian-gw ==="
	CONTROL_PLANE_IMAGE=$(CONTROL_PLANE_IMAGE) DATA_PLANE_IMAGE=$(DATA_PLANE_IMAGE) DASHBOARD_IMAGE=$(DASHBOARD_IMAGE) TIMEOUT=$(CONFORMANCE_TIMEOUT)s scripts/ci/deploy-kind-conformance.sh
	@echo "=== Running conformance tests ==="
	go test -tags=conformance -count=1 -v -timeout 30m ./conformance/ -args -gateway-class nantian-gw
	@echo "=== Cleaning up ==="
	kind delete cluster --name $(CLUSTER_NAME)

e2e-smoke:
	CLUSTER_NAME=nantian-e2e ./test/e2e/smoke/run.sh
