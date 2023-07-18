#!/usr/bin/env bash

# Copyright 2023 Stefan Prodan
# SPDX-License-Identifier: Apache-2.0

set -o errexit

cluster_name="timoni"
reg_name='timoni-registry'

kind delete cluster --name ${cluster_name}

docker rm -f ${reg_name}
