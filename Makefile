PROTO_GO_OUT := controlplane/internal/gen

.PHONY: build fmt test-unit test-race test-coverage bench lint test-skew test-e2e test-conformance test-targeted test-security doctor clean-artifacts helm-validate

build:
	cd controlplane && CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath ./...

fmt:
	cd controlplane && gofmt -w $$(find . -name '*.go')

test-unit:
	cd controlplane && go test ./...

test-race:
	cd controlplane && go test -race ./...

test-coverage:
	cd controlplane && go test -coverprofile=coverage.out ./...

bench:
	cd controlplane && go test -bench=. ./...

lint:
	cd controlplane && golangci-lint run

test-skew:
	./scripts/run-skew-validation.sh

test-e2e:
	./tests/e2e/run-kind.sh

test-conformance:
	./tests/conformance/run.sh

test-targeted:
	./scripts/run-targeted-validation.sh

test-security:
	./scripts/run-security-scans.sh

doctor:
	./scripts/doctor.sh

clean-artifacts:
	./scripts/clean-artifacts.sh

helm-validate:
	helm lint deploy/helm/aether-gateway/
	helm template aether-gateway deploy/helm/aether-gateway/ > /dev/null
	@echo "Helm chart validation passed"