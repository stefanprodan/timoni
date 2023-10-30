# Quickstart Guide

This guide shows you the basics of Timoni.
You'll deploy a demo application on Kubernetes using a Timoni module
published in a container registry.

## Prerequisites

To follow this guide you'll need:

* A Kubernetes cluster
* Timoni installed on a system with writable `/tmp`

Install Timoni with:

```shell
brew install stefanprodan/tap/timoni
```

For other methods,
see the [installation guide](install.md).

## Install a module

To install a [Timoni module](module.md) on a Kubernetes cluster,
you have to specify the container registry address and the version of a module.

!!! tip "Modules and Instances"

    If you are familiar with Helm,
    a Timoni **module** is the equivalent of a **chart**,
    and a Timoni **instance** is the equivalent of a Helm **release**.
    To learn more about modules and instances, please see the [concepts doc](concepts.md).

For example, to install the latest stable version of [podinfo](https://github.com/stefanprodan/podinfo)
in a new namespace:

```console
$ timoni -n test apply podinfo oci://ghcr.io/stefanprodan/modules/podinfo --version latest
pulling oci://ghcr.io/stefanprodan/modules/podinfo:latest
using module timoni.sh/podinfo version 6.5.3
installing podinfo in namespace test
Namespace/test created
ServiceAccount/test/podinfo created
Service/test/podinfo created
Deployment/test/podinfo created
waiting for 3 resource(s) to become ready...
all resources are ready
```

## List and inspect instances

You can list all instances in a cluster with `timoni ls -A`.

To get more information on an instance, you can use the `timoni inspect` sub-commands:

```console
$ timoni -n test inspect module podinfo
name: timoni.sh/podinfo
version: 6.5.3
repository: oci://ghcr.io/stefanprodan/modules/podinfo
digest: sha256:0e1f9cfbd020230e4e9f8c3b14bc728df932793d02d4f0a26512737af6df2dc8
```

To learn more about the available commands, use `timoni inspect --help`.

To see the status of the Kubernetes resources managed by an instance:

```shell
timoni -n test status podinfo 
```

## Configure a module instance

To customise an instance, you can supply the configuration using values files.

For example, to set the [QoS](https://kubernetes.io/docs/tasks/configure-pod-container/quality-service-pod/)
class to guaranteed, create a `qos-values.cue` file that sets the resources limits equal to the requests:

```cue
values: {
	resources: requests: {
		cpu:    "100m"
		memory: "128Mi"
	}
	resources: limits: resources.requests
}
```

Apply the config to the podinfo module to perform an upgrade:

```shell
timoni -n test apply podinfo \
  oci://ghcr.io/stefanprodan/modules/podinfo \
  --values qos-values.cue
```

Before running an upgrade, you can review the changes that will
be made on the cluster with `timoni apply --dry-run --diff`.

To learn more about all the available apply options, use `timoni apply --help`.

## Uninstall a module instance

To uninstall an instance and delete all the managed Kubernetes resources:

```shell
timoni -n test delete podinfo --wait
```

## Bundling instances

For deploying complex applications to production, it is recommended to use
Timoni [Bundles](bundle.md).

A Timoni Bundle is a CUE file for defining a group of instances together
with their values and module references.

The following is an example of a Bundle that defines a Redis master-replica cluster
and a podinfo instance connected to the Redis instance.

```cue
bundle: {
	apiVersion: "v1alpha1"
	name:       "podinfo"
	instances: {
		redis: {
			module: {
				url:     "oci://ghcr.io/stefanprodan/modules/redis"
				version: "7.2.2"
			}
			namespace: "podinfo"
			values: maxmemory: 256
		}
		podinfo: {
			module: url:     "oci://ghcr.io/stefanprodan/modules/podinfo"
			module: version: "6.5.3"
			namespace: "podinfo"
			values: caching: {
				enabled:  true
				redisURL: "tcp://redis:6379"
			}
		}
	}
}
```

To apply the above bundle on a cluster, save the file as `podinfo.bundle.cue` and run:

```shell
timoni bundle apply -f podinfo.bundle.cue
```

To list all the instances managed by a bundle:

```shell
timoni list -A --bundle podinfo
```

To delete all the instances defined in a bundle:

```shell
timoni bundle delete -f podinfo.bundle.cue
```

To learn more about bundles, please see the [Bundle API](bundle.md)
and the [Bundle distribution](bundle-distribution.md) documentation.
