#!/usr/bin/env bash

# Copyright 2023 Stefan Prodan
# SPDX-License-Identifier: Apache-2.0

set -o errexit

CLUSTER_NAME="${CLUSTER_NAME:=timoni}"
reg_name='timoni-registry'

kind delete cluster --name ${CLUSTER_NAME}

docker rm -f ${reg_name}
