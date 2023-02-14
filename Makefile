# timoni test, build, install makefile

.ONESHELL:
.SHELLFLAGS += -e

all: test build

build:
	CGO_ENABLED=0 go build -o ./bin/timoni ./cmd/timoni

.PHONY: test
test: tidy fmt vet
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

TESTMOD=podinfo ./examples/podinfo
TESTVAL_HA=-f ./examples/values/podinfo-ha-values.cue
TESTVAL_ING=-f ./examples/values/podinfo-ingress-values.cue
test-template:
	./bin/timoni template $(TESTMOD) $(TESTVAL_HA)

test-install:
	./bin/timoni install $(TESTMOD) $(TESTVAL_HA)

test-diff:
	./bin/timoni upgrade --dry-run --diff $(TESTMOD) $(TESTVAL_ING)

test-upgrade:
	./bin/timoni upgrade $(TESTMOD) $(TESTVAL_HA) $(TESTVAL_ING)

test-uninstall:
	./bin/timoni uninstall $(TESTMOD)

.PHONY: help
help:  ## Display this help menu
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
