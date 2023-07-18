#!/usr/bin/env bash

# Copyright 2023 Stefan Prodan
# SPDX-License-Identifier: Apache-2.0

set -o errexit

reg_localhost_port='5555'
repo_root=$(git rev-parse --show-toplevel)

PODINFO_VER=$(cat $repo_root/examples/podinfo/templates/config.cue | awk '/tag:/ {print $2}' | tr -d '*"')
timoni mod push $repo_root/examples/podinfo oci://localhost:${reg_localhost_port}/modules/podinfo -v ${PODINFO_VER} --latest \
		--source https://github.com/stefanprodan/podinfo \
		-a 'org.opencontainers.image.description=A timoni.sh module for deploying Podinfo.' \
		-a 'org.opencontainers.image.documentation=https://github.com/stefanprodan/timoni/blob/main/examples/podinfo/README.md'

REDIS_VER=$(cat $repo_root/examples/redis/templates/config.cue | awk '/tag:/ {print $2}' | tr -d '*"')
timoni mod push $repo_root/examples/redis oci://localhost:${reg_localhost_port}/modules/redis -v ${REDIS_VER} --latest \
		--source https://github.com/stefanprodan/timoni/tree/main/examples/redis  \
		-a 'org.opencontainers.image.description=A timoni.sh module for deploying Redis master-replica clusters.' \
		-a 'org.opencontainers.image.documentation=https://github.com/stefanprodan/timoni/blob/main/examples/redis/README.md'
