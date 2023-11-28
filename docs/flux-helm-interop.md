# Helm interoperability with Flux

[Flux AIO](flux-aio.md) can be used as a bridge between Timoni and Helm, enabling Timoni
to orchestrate Helm chart deployments by leveraging Flux's declarative Helm APIs
such as `HelmRepository` and `HelmRelease` kinds.

With Flux, Timoni users can take full advantage of existing Helm charts.
Timoni [Bundles](bundle.md) can refer to Helm charts hosted on Helm HTTPS and OCI repositories,
and supports setting Helm release values in the same way you would do for Timoni instances.

## Install Flux helm-controller

Install Flux helm-controller on a Kubernetes cluster with:

```cue
bundle: {
    apiVersion: "v1alpha1"
    name:       "flux-aio"
    instances: {
        "flux": {
            module: url: "oci://ghcr.io/stefanprodan/modules/flux-aio"
            namespace: "flux-system"
            values: {
                controllers: {
                    helm: enabled:         true
                    kustomize: enabled:    false
                    notification: enabled: false
                }
                hostNetwork:     false
                securityProfile: "privileged"
            }
        }
    }
}
```

Apply the bundle with:

```shell
timoni bundle apply -f flux-aio.cue
```

## Deploy Helm charts

To deploy Helm charts on clusters with Flux installed, you'll be using
the [flux-helm-release](https://github.com/stefanprodan/flux-aio/tree/main/modules/flux-helm-release)
Timoni module. This module generates Flux `HelmRepository` and `HelmRelease` objects and allows
the configuration of the Helm repository HTTP/S or OCI URL, auth token, chart name, and Helm release values.

### Public repositories

Example of deploying `cert-manager` and `ingress-nginx` Helm charts to a Kubernetes cluster:

```cue
bundle: {
	apiVersion: "v1alpha1"
	name:       "cluster-addons"
	instances: {
		"cert-manager": {
			module: url: "oci://ghcr.io/stefanprodan/modules/flux-helm-release"
			namespace: "cert-manager"
			values: {
				repository: url: "https://charts.jetstack.io"
				chart: {
					name:    "cert-manager"
					version: "1.x"
				}
				helmValues: {
					installCRDs: true
				}
			}
		}
		"ingress-nginx": {
			module: url: "oci://ghcr.io/stefanprodan/modules/flux-helm-release"
			namespace: "ingress-nginx"
			values: {
				repository: url: "https://kubernetes.github.io/ingress-nginx"
				chart: {
					name:    "ingress-nginx"
					version: "4.x"
				}
				helmValues: {
					controller: service: type: "NodePort"
				}
			}
		}
	}
}
```

Apply the bundle with:

```shell
timoni bundle apply -f cluster-addons.cue
```

Timoni will create the Flux Helm repositories, will wait for Flux to install
the `cert-manager` release, then will proceed with the `ingress-nginx` installation.

After the releases are installed, Flux will scan for new chart versions every hour,
and will upgrade a release if a new chart version is found. To disable the automated
upgrade, you can set a fix version for each chart under `values: chart: version`.

### Private repositories

When using Helm charts from a private Helm HTTPS or OCI repository, you can
provide the auth credentials in the Bundle using Timoni runtime attributes.

Example of deploying the `podinfo` Helm chart from GitHub Container Registry
using a GitHub PAT for auth:

```cue
bundle: {
	apiVersion: "v1alpha1"
	name:       "podinfo"
	instances: {
		"podinfo": {
			module: url: "oci://ghcr.io/stefanprodan/modules/flux-helm-release"
			namespace: "podinfo"
			values: {
				repository: {
					url: "oci://ghcr.io/stefanprodan/charts"
					auth: {
						username: "flux"
						password: string @timoni(runtime:string:GITHUB_TOKEN)
					}
				}
				chart: {
					name:    "podinfo"
					version: "*"
				}
				helmValues: {
					logLevel: "info"
				}
			}
		}
	}
}
```

Assuming the `GITHUB_TOKEN` is set in your environment, apply the bundle
using the `--runtime-from-env` flag and Timoni will fill in the token value:

```shell
timoni bundle apply -f podinfo.cue --runtime-from-env
```

Timoni will create a Kubernetes Secret with the Helm credentials, and will
configure Flux to use the Secret when pulling the Helm OCI charts from the
container registry.
