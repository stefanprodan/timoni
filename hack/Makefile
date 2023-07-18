# Copyright 2023 Stefan Prodan
# SPDX-License-Identifier: Apache-2.0

# Timoni local dev
# Requirements: docker, kind, kubectl, timoni

.ONESHELL:
.SHELLFLAGS += -e

REPOSITORY_ROOT := $(shell git rev-parse --show-toplevel)

.PHONY: tools
tools: # Install required tools with Homebrew
	brew bundle

.PHONY: up
up: # Start a local Kind clusters and a container registry on port 5555
	$(REPOSITORY_ROOT)/hack/local/kind-up.sh

.PHONY: down
down: # Teardown the Kind cluster and registry
	$(REPOSITORY_ROOT)/hack/local/kind-down.sh

.PHONY: push
push: # Push the example modules to the local registry
	$(REPOSITORY_ROOT)/hack/local/mod-push.sh

.PHONY: deploy
deploy: # Deploy a bundle using the modules from the local registry
	timoni bundle apply -f $(REPOSITORY_ROOT)/hack/local/podinfo.bundle.cue

.PHONY: help
help:  ## Display this help menu
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
