#!/usr/bin/env bash

# Copyright 2023 Stefan Prodan
# SPDX-License-Identifier: Apache-2.0

set -o errexit

reg_localhost_port='5555'
repo_root=$(git rev-parse --show-toplevel)

crane copy ghcr.io/stefanprodan/modules/podinfo localhost:${reg_localhost_port}/modules/podinfo -a
crane copy ghcr.io/stefanprodan/modules/redis localhost:${reg_localhost_port}/modules/redis -a
crane copy ghcr.io/stefanprodan/timoni/minimal localhost:${reg_localhost_port}/modules/nginx -a
