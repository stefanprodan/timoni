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

To install a [Timoni module](index.md#timoni-modules) on a Kubernetes cluster,
you have to specify the container registry address and the version of a module.

!!! tip "Modules and Instances"

    If you are familiar with Helm,
    a Timoni **module** is the equivalent of a **chart**,
    and a Timoni **instance** is the equivalent of a Helm **release**.
    To learn more about modules and instances, please see the [concepts section](index.md#concepts).

For example, to install [podinfo](https://github.com/stefanprodan/podinfo)
in a new namespace:

```console
$ timoni -n test apply podinfo oci://ghcr.io/stefanprodan/modules/podinfo --version 6.3.3
pulling oci://ghcr.io/stefanprodan/modules/podinfo:6.3.3
installing podinfo/test
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
digest: sha256:a6763971b24afca01176d796657de96055952c0857317a99f0b2e06d43fdb10b
name: timoni.sh/podinfo
repository: oci://ghcr.io/stefanprodan/modules/podinfo
version: 6.3.3
```

To learn more about the available commands, use `timoni inspect --help`.

To see the status of the Kubernetes resources managed by an instance:

```shell
timoni -n test status podinfo 
```

## Configure a module instance

To customise an instance, you can supply the configuration values using `values.cue` files.

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
  oci://ghcr.io/stefanprodan/modules/podinfo -v 6.3.4 \
  --values qos-values.cue
```

Before running an upgrade, you can review the changes that will
be made on the cluster with `timoni apply --dry-run --diff`.

To learn more about all the available apply options, use `timoni apply --help`.

In the values files you can use arithmetic operations,
string interpolation and everything else that CUE std lib supports.
For example, to set the resources limits to 2x requests:

```cue
values: {
	_mcpu: 500
	_mem:  256
	resources: {
		requests: {
			cpu:    "\(_mcpu)m"
			memory: "\(_mem)Mi"
		}
		limits: {
			cpu:    "\(_mcpu*2)m"
			memory: "\(_mem*2)Mi"
		}
	}
}
```

## Uninstall a module instance

To uninstall an instance and delete all the managed Kubernetes resources:

```shell
timoni -n test delete podinfo --wait
```
