#!/usr/bin/env bash

# Copyright 2023 Stefan Prodan
# SPDX-License-Identifier: Apache-2.0

set -o errexit

app="$1"

crane digest cgr.dev/chainguard/$app

cosign download attestation --platform=linux/amd64 --predicate-type=https://spdx.dev/Document cgr.dev/chainguard/$app \
| jq -r .payload | base64 -d | jq -r '.predicate.packages[] | "\(.name) \(.versionInfo)"' \
| grep $app | awk '{ print $2 }'
