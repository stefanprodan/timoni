#!/usr/bin/env bash

# Copyright 2023 Stefan Prodan
# SPDX-License-Identifier: Apache-2.0

set -o errexit

CLUSTER_NAME="${CLUSTER_NAME:=timoni}"
reg_name='timoni-registry'
reg_localhost_port='5555'
reg_cluster_port='5000'

install_cluster() {
cat <<EOF | kind create cluster --name ${CLUSTER_NAME} --wait 5m --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
  - |-
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:${reg_localhost_port}"]
      endpoint = ["http://${reg_name}:${reg_cluster_port}"]
nodes:
  - role: control-plane
    kubeadmConfigPatches:
      - |
        kind: InitConfiguration
        nodeRegistration:
          kubeletExtraArgs:
            node-labels: "ingress-ready=true"
EOF
}

register_registry() {
cat <<EOF | kubectl apply --server-side -f-
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${reg_localhost_port}"
    hostFromContainerRuntime: "${reg_name}:${reg_cluster_port}"
    hostFromClusterNetwork: "${reg_name}:${reg_cluster_port}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF
}

# Create a registry container
if [ "$(docker inspect -f '{{.State.Running}}' "${reg_name}" 2>/dev/null || true)" != 'true' ]; then
  echo "starting Docker registry on localhost:${reg_localhost_port}"
  docker run -d --restart=always -p "127.0.0.1:${reg_localhost_port}:${reg_cluster_port}" \
    --name "${reg_name}" registry:2
fi

# Create a cluster with the local registry enabled
if [ "$(kind get clusters | grep ${CLUSTER_NAME})" != "${CLUSTER_NAME}" ]; then
  install_cluster
  register_registry
else
  echo "cluster ${CLUSTER_NAME} exists"
fi

# Connect the registry to the cluster network
if [ "$(docker inspect -f='{{json .NetworkSettings.Networks.kind}}' "${reg_name}")" = 'null' ]; then
  echo "connecting the Docker registry to the cluster network"
  docker network connect "kind" "${reg_name}"
fi
