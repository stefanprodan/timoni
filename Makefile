# timoni test, build, install makefile

.ONESHELL:
.SHELLFLAGS += -e

all: test build

build:
	CGO_ENABLED=0 go build -o ./bin/timoni ./cmd/timoni

.PHONY: test
test: tidy fmt vet
	go test ./... -coverprofile cover.out

run: build
	./bin/timoni build podinfo ./examples/podinfo/module/ -p main -f ./examples/podinfo/my-values.cue -o json | kubectl apply -f- --dry-run=server

dryrun: build
	./bin/timoni apply podinfo ./examples/podinfo/module/ -f ./examples/podinfo/my-values.cue --dry-run

diff: build
	./bin/timoni apply --dry-run --diff podinfo ./examples/podinfo/module/ -f ./examples/podinfo/my-values.cue

apply: build
	./bin/timoni apply podinfo ./examples/podinfo/module/ -f ./examples/podinfo/my-values.cue

tidy:
	rm -f go.sum; go mod tidy -compat=1.19

fmt:
	go fmt ./...

vet:
	go vet ./...

cuevet:
	cue fmt ./...
	cd ./examples/podinfo/module/ && cue vet --all-errors --concrete ./...

.PHONY: install
install:
	go install ./cmd/timoni

.PHONY: help
help:  ## Display this help menu
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
