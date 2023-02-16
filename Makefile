# timoni test, build, install makefile

.ONESHELL:
.SHELLFLAGS += -e

# Repository root based on Git metadata
REPOSITORY_ROOT := $(shell git rev-parse --show-toplevel)
BIN_DIR := $(REPOSITORY_ROOT)/bin

# API gen tool
CONTROLLER_GEN_VERSION ?= v0.11.1

all: test build

build:
	CGO_ENABLED=0 go build -o ./bin/timoni ./cmd/timoni

.PHONY: test
test: tidy generate fmt vet
	go test ./... -coverprofile cover.out

tidy:
	rm -f go.sum; go mod tidy -compat=1.19

fmt:
	go fmt ./...

vet:
	go vet ./...

.PHONY: install
install:
	go install ./cmd/timoni


generate: controller-gen  ## Generate API code
	cd api; $(CONTROLLER_GEN) object:headerFile="license.go.txt" paths="./..."

CONTROLLER_GEN = $(BIN_DIR)/controller-gen
.PHONY: controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION))

TESTMOD=podinfo ./examples/podinfo
TESTINST=podinfo
TESTVAL_HA=-f ./examples/podinfo-values/ha-values.cue
TESTVAL_ING=-f ./examples/podinfo-values/ingress-values.cue
test-template:
	./bin/timoni template $(TESTMOD) $(TESTVAL_HA)

test-install:
	./bin/timoni install $(TESTMOD) $(TESTVAL_HA)

test-diff:
	./bin/timoni upgrade --dry-run --diff $(TESTMOD) $(TESTVAL_ING)

test-upgrade:
	./bin/timoni upgrade $(TESTMOD) $(TESTVAL_HA) $(TESTVAL_ING)

test-list:
	./bin/timoni list $(TESTINST)

test-inspect:
	./bin/timoni inspect values $(TESTINST)
	./bin/timoni inspect resources $(TESTINST)

test-uninstall:
	./bin/timoni uninstall $(TESTMOD)

test-lint:
	./bin/timoni lint ./examples/podinfo

lint: test-lint

docs: build
	./bin/timoni docgen

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
