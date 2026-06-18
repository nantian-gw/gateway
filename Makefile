.PHONY: build test benchmarks conformance e2e-smoke

CLUSTER_NAME ?= nantian-conformance
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
	kind create cluster --name $(CLUSTER_NAME) --wait 5m
	kubectl wait --for=condition=ready node --all --timeout=2m
	@echo "=== Installing Gateway API CRDs ==="
	GATEWAY_API_CHANNEL=experimental scripts/ci/install-gateway-api-crds.sh
	@echo "=== Deploying nantian-gw ==="
	kustomize build deploy/kubernetes/overlays/kind-conformance --load-restrictor LoadRestrictionsNone | kubectl apply -f -
	kubectl wait --for=condition=ready pod --all -n nantian-gw --timeout=180s
	@echo "=== Running conformance tests ==="
	go test -tags=conformance -count=1 -v -timeout 30m ./conformance/
	@echo "=== Cleaning up ==="
	kind delete cluster --name $(CLUSTER_NAME)

e2e-smoke:
	CLUSTER_NAME=nantian-e2e ./test/e2e/smoke/run.sh
