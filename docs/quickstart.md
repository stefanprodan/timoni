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

=== "command"

    ```shell
    timoni -n test apply podinfo oci://ghcr.io/stefanprodan/modules/podinfo
    ```

=== "output"

    ```text
    pulling oci://ghcr.io/stefanprodan/modules/podinfo:latest
    using module timoni.sh/podinfo version 6.5.4
    installing podinfo in namespace test
    Namespace/test created
    ServiceAccount/test/podinfo created
    Service/test/podinfo created
    Deployment/test/podinfo created
    waiting for 3 resource(s) to become ready...
    all resources are ready
    ```

The apply command pulls the module from the container registry,
creates the Kubernetes resources in the specified namespace,
and waits for all resources to become ready.

To learn more about all the available apply options, use `timoni apply --help`.

## List and inspect instances

You can list all instances in a cluster with:

=== "command"

    ```shell
    timoni list -A
    ```

=== "output"

    ```text
    NAME   	NAMESPACE	MODULE                                    	VERSION	LAST APPLIED        	BUNDLE 
    podinfo	test     	oci://ghcr.io/stefanprodan/modules/podinfo	6.5.4  	2024-01-20T19:51:17Z	- 
    ```

To see the status of the Kubernetes resources managed by an instance:

=== "command"

    ```shell
    timoni -n test status podinfo
    ```

=== "output"

    ```text
    last applied 2024-01-20T19:51:17Z
    module oci://ghcr.io/stefanprodan/modules/podinfo:6.5.4
    digest sha256:1dba385f9d56f9a79e5b87344bbec1502bd11f056df51834e18d3e054de39365
    container image ghcr.io/curl/curl-container/curl-multi:master
    container image ghcr.io/stefanprodan/podinfo:6.5.4
    ServiceAccount/test/podinfo Current - Resource is current
    Service/test/podinfo Current - Service is ready
    Deployment/test/podinfo Current - Deployment is available. Replicas: 1
    ```

To get more information on an instance, you can use the `timoni inspect` sub-commands.

For example, to list the module URL, version and OCI digest of the podinfo instance:

=== "command"

    ```shell
    timoni -n test inspect module podinfo
    ```

=== "output"

    ```text
    digest: sha256:1dba385f9d56f9a79e5b87344bbec1502bd11f056df51834e18d3e054de39365
    name: timoni.sh/podinfo
    repository: oci://ghcr.io/stefanprodan/modules/podinfo
    version: 6.5.4
    ```

To learn more about the available commands, use `timoni inspect --help`.

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

=== "command"

    ```shell
    timoni -n test apply podinfo oci://ghcr.io/stefanprodan/modules/podinfo \
      --values qos-values.cue
    ```

=== "output"

    ```text
    pulling oci://ghcr.io/stefanprodan/modules/podinfo:latest
    using module timoni.sh/podinfo version 6.5.4
    upgrading podinfo in namespace test
    ServiceAccount/test/podinfo unchanged
    Service/test/podinfo unchanged
    Deployment/test/podinfo configured
    resources are ready
    ```

Before running an upgrade, you can review the changes that will
be made on the cluster with `timoni apply --dry-run --diff`.

## Uninstall a module instance

To uninstall an instance and delete all the managed Kubernetes resources:

=== "command"

    ```shell
    timoni -n test delete podinfo
    ```

=== "output"

    ```text
    deleting 3 resource(s)...
    Deployment/test/podinfo deleted
    Service/test/podinfo deleted
    ServiceAccount/test/podinfo deleted
    all resources have been deleted
    ```

By default, the delete command will wait for all the resources to be removed.
To skip waiting, use the `--wait=false` flag.

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
				version: "7.2.4"
			}
			namespace: "podinfo"
			values: maxmemory: 256
		}
		podinfo: {
			module: url:     "oci://ghcr.io/stefanprodan/modules/podinfo"
			module: version: "6.5.4"
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
