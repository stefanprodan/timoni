# timoni test, build, install makefile

.ONESHELL:
.SHELLFLAGS += -e

# Repository root based on Git metadata
REPOSITORY_ROOT := $(shell git rev-parse --show-toplevel)
BIN_DIR := $(REPOSITORY_ROOT)/bin

# API gen tool
CONTROLLER_GEN_VERSION ?= v0.11.3

# Kubernetes env test
ENVTEST_ARCH?=amd64
ENVTEST_KUBERNETES_VERSION?=1.26

all: test build

DEV_VERSION?=0.0.0-$(shell git rev-parse --short HEAD).$(shell date +%s)
build: ## Build the CLI binary.
	CGO_ENABLED=0 go build -ldflags="-s -w -X main.VERSION=$(DEV_VERSION)" -o ./bin/timoni ./cmd/timoni

.PHONY: test
test: tidy generate fmt vet install-envtest ## Run the Go tests.
	KUBEBUILDER_ASSETS=$(KUBEBUILDER_ASSETS) go test ./... -coverprofile cover.out

tidy: ## Tidy Go modules.
	rm -f go.sum; go mod tidy -compat=1.20

fmt: ## Format Go code.
	go fmt ./...

vet: ## Vet Go code.
	go vet ./...

lint-samples: build
	./bin/timoni mod lint ./examples/podinfo
	cue fmt ./examples/podinfo-values/
	./bin/timoni mod lint ./cmd/timoni/testdata/module
	cue fmt ./cmd/timoni/testdata/module-values/
	./bin/timoni mod lint ./internal/engine/testdata/module
	cue fmt ./internal/engine/testdata/module-values

PODINFO_VER=$(shell cat ./examples/podinfo/templates/config.cue | awk '/tag:/ {print $$2}' | tr -d '*"')
push-podinfo: build
	./bin/timoni mod push ./examples/podinfo oci://ghcr.io/stefanprodan/modules/podinfo -v $(PODINFO_VER) --latest --source https://github.com/stefanprodan/podinfo

.PHONY: install
install: ## Build and install the CLI binary.
	go install ./cmd/timoni

generate: controller-gen ## Generate API code.
	cd api; $(CONTROLLER_GEN) object:headerFile="license.go.txt" paths="./..."

docs: build
	./bin/timoni docgen

CONTROLLER_GEN=$(BIN_DIR)/controller-gen
.PHONY: controller-gen
controller-gen:
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION))

KUBEBUILDER_ASSETS?="$(shell $(ENVTEST) --arch=$(ENVTEST_ARCH) use -i $(ENVTEST_KUBERNETES_VERSION) --bin-dir=$(BIN_DIR) -p path)"
install-envtest: setup-envtest ## Install controller-runtime envtest.
	$(ENVTEST) use $(ENVTEST_KUBERNETES_VERSION) --arch=$(ENVTEST_ARCH) --bin-dir=$(BIN_DIR)

ENVTEST=$(BIN_DIR)/setup-envtest
.PHONY: envtest
setup-envtest:
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

# go-install-tool will 'go install' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-install-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

.PHONY: help
help:  ## Display this help menu
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
