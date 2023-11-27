# Flux AIO Distribution

Flux All-In-One is a lightweight [Flux CD](https://fluxcd.io) distribution made
with Timoni for running the GitOps Toolkit controllers as a single deployable unit.

!!! tip "Helm charts interoperability"
    
    Flux AIO can be used as a bridge between Timoni and Helm, enabling Timoni to orchestrate
    Helm chart deployments by leveraging Flux's declarative Helm APIs
    such as `HelmRepository` and `HelmRelease`.

## Specifications

This distribution is optimized for running Flux on:

- Edge clusters with limited CPU and memory resources
- Bare clusters without a CNI plugin installed
- Clusters where plain HTTP communication is not allowed between pods
- Serverless clusters for cost optimisation (EKS Fargate, GKE Autopilot)

Timoni modules part of this distribution:

- [flux-aio](https://github.com/stefanprodan/flux-aio/tree/main/modules/flux-aio) - A module for deploying 
  Flux core components such as source-controller, helm-controller, kustomize-controller and notification-controller.
- [flux-git-sync](https://github.com/stefanprodan/flux-aio/tree/main/modules/flux-git-sync) - A module for 
  configuring Flux to deploy workloads from Git repositories.
- [flux-helm-release](https://github.com/stefanprodan/flux-aio/tree/main/modules/flux-helm-release) - A module for
  configuring Flux to deploy Helm releases from charts hosted on Helm HTTP/S and OCI repositories.
- [flux-tenant](https://github.com/stefanprodan/flux-aio/tree/main/modules/flux-tenant) - A module for
  onboarding tenants with limited access on Flux-managed clusters.

## Flux installation

To deploy Flux on Kubernetes clusters, you'll be using
the flux-aio module and a [Bundle file](bundle.md)
where you'll define the configuration of the Flux controllers and their settings.

=== "Default installation"

    Install Flux with default settings on a Kubernetes cluster:
    
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
                        kustomize: enabled:    true
                        notification: enabled: true
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

    You can fine tune the Flux installation using various options listed in the flux-aio module
    [readme](https://github.com/stefanprodan/flux-aio/tree/main/modules/flux-aio/README.md#configuration).

=== "Bare Metal"
    
    To install Flux on a bare metal cluster, you need to set `hostNetwork: true` in the 
    flux-aio module values.
    
    For example, to deploy Flux and configure it to install the latest version of the Cilium CNI:
    
    ```cue
    bundle: {
        apiVersion: "v1alpha1"
        name:       "flux-aio"
        instances: {
            "flux": {
                module: url: "oci://ghcr.io/stefanprodan/modules/flux-aio"
                namespace: "flux-system"
                values: {
                    hostNetwork:     true
                    securityProfile: "privileged"
                }
            }
            "cilium": {
                module: url: "oci://ghcr.io/stefanprodan/modules/flux-helm-release"
                namespace: "flux-system"
                values: {
                    repository: url: "https://helm.cilium.io"
                    chart: {
                        name:    "cilium"
                        version: "*"
                    }
                    helmValues: {
                        operator: replicas: 1
                        ipam: mode:         "kubernetes"
                    }
                    sync: targetNamespace: "kube-system"
                }
            }
        }
    }
    ```

    The Flux pod binds to the following ports on the host network:
    
    - `9292` notification-controller webhook receiver endpoint
    - `9690` notification-controller events receiver endpoint
    - `9790` source-controller storage endpoint
    - `9791-9799` metrics, liveness and readiness endpoints

    Note that on clusters without `kube-proxy`, you'll have to add the following env vars to values:
    
    ```cue
    values: env: {
        "KUBERNETES_SERVICE_HOST": "<host>"
        "KUBERNETES_SERVICE_PORT": "<port>"
    }
    ```

=== "Cloud"

    When installing Flux on a managed Kubernetes cluster, the host network can be disabled
    if the cloud vendor has already setup a CNI for you. You can also configure
    persistent storage for Flux artifacts cache to speed up the startup after a pod eviction.
    
    To grant Flux access to cloud resources such as container registries (for pulling OCI artifacts)
    or KMS (for secretes decryption), you can use Kubernetes Workload Identity to bind the `flux`
    service account from the `flux-system` namespace to an IAM role.
    
    For example, on an EKS cluster with IRSA enabled, grant Flux access to ECR
    by specified an AWS role ARN:
    
    ```cue
    bundle: {
        apiVersion: "v1alpha1"
        name:       "flux-aio"
        instances: {
            "flux": {
                module: url: "oci://ghcr.io/stefanprodan/modules/flux-aio"
                namespace: "flux-system"
                values: {
                    hostNetwork:     false
                    securityProfile: "privileged"
                    workload: {
                        identity: "arn:aws:iam::111122223333:role/my-role"
                        provider: "aws"
                    }
                    persistence: {
                        enabled:      true
                        storageClass: "standard"
                        size:         "8Gi"
                    }
                }
            }
        }
    }
    ```
    
    For Azure Workload Identity, the type must be set to `azure` and the identity set to the Azure Client ID.
    
    For Google Cloud, the type must be set to `gcp` and the identity set to the GCP Identity Name.

## Flux Git sync configuration

To configure Flux to deploy workloads from a Git repository,
you'll be using the flux-git-sync module.

This module generates Flux `GitRepository` and `Kustomization` objects and allows
the configuration of the Git HTTPS URL, auth token, branch, path, interval, health checks.

=== "Public repository"

    To configure Flux to sync with a public Git repository, you have to specify the
    Git HTTPS address of the repository, the Git ref (can be a branch or tag)
    and the path to the Kubernetes manifests or to the Kustomize overlay.
    
    For example, to sync the [podinfo](https://github.com/stefanprodan/podinfo)
    Kustomize overlay to the `default` namespace:
 
    ```cue
    bundle: {
        apiVersion: "v1alpha1"
        name:       "podinfo"
        instances: {
            "podinfo": {
                module: url: "oci://ghcr.io/stefanprodan/modules/flux-git-sync"
                namespace: "flux-system"
                values: {
                    git: {
                        url:  "https://github.com/stefanprodan/podinfo"
                        ref:  "refs/heads/master"
                        path: "./kustomize"
                    }
                    sync: targetNamespace: "default"
                }
            }
        }
    }
    ```
    
    Apply the bundle with:
    
    ```shell
    timoni bundle apply -f podinfo.cue
    ```
    
    You can fine tune the sync using the options listed in the flux-git-sync module
    [readme](https://github.com/stefanprodan/flux-aio/tree/main/modules/flux-gitsync/README.md#configuration).

=== "Private repository"

    To configure Flux to sync with a private Git repository,
    you can specify a Git token (GitHub PAT, GitLab deploy token, BitBucket token, etc).
    
    To avoid storing sensitive information in your bundle files,
    Timoni can read values from environment variable.
    
    For example, to sync podinfo from your own private repo:
    
    ```cue
    bundle: {
        apiVersion: "v1alpha1"
        name:       "podinfo"
        instances: {
            "podinfo": {
                module: url: "oci://ghcr.io/stefanprodan/modules/flux-git-sync"
                namespace: "flux-system"
                values: {
                    git: {
                        token: string @timoni(runtime:string:GITHUB_TOKEN)
                        url:   "https://github.com/my-org/my-private-repo"
                        ref:   "refs/heads/master"
                        path:  "./kustomize"
                    }
                    sync: targetNamespace: "default"
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

=== "Self-hosted repository"

    When using a self-hosted Git server such as GitLab, the self-signed TLS
    Certificate Authority must be specified for Flux to be able to sync the repository.
    
    For example, to sync podinfo from a self-hosted private repo:
    
    ```cue
    bundle: {
        apiVersion: "v1alpha1"
        name:       "podinfo"
        instances: {
            "podinfo": {
                module: url: "oci://ghcr.io/stefanprodan/modules/flux-git-sync"
                namespace: "flux-system"
                values: {
                    git: {
                        ca:    string @timoni(runtime:string:GIT_CA)
                        token: string @timoni(runtime:string:GIT_TOKEN)
                        url:   "https://gitlab.internal/my-group/my-repo"
                        ref:   "refs/heads/master"
                        path:  "./kustomize"
                    }
                    sync: targetNamespace: "default"
                }
            }
        }
    }
    ```
    
    Export the `GIT_TOKEN` and `GIT_CA` env vars, then apply the bundle:
    
    ```shell
    export GIT_CA=$(cat ca.crt)
    export GIT_TOKEN="your-token"
    
    timoni bundle apply -f podinfo.cue --runtime-from-env
    ```
    
    Note that the CA certificate must be PEM-encoded.

## Flux multi-tenancy configuration

To enable Flux [multi-tenancy lockdown](https://fluxcd.io/flux/installation/configuration/multitenancy/),
you can set `securityProfile: "restricted"` in the flux-aio values.

With the restricted profile, Flux Kustomizations and HelmReleases
can't create cluster-wide resources (CRDs, Namespaces, ClusterRoleBindings, etc)
unless they are deployed in the `flux-system` namespace.
The `flux-system` namespace, like `kube-system`, is reserved to cluster admins.

### On-board tenants

To configure Flux to deploy workloads from a tenant repository,
you'll be using the `flux-tenant` and  `flux-git-sync` Timoni modules.

The `flux-tenant` module generates the tenant's Kubernetes namespace
and RBAC (service account & role binding) that constrains Flux to be able
to deploy applications only in that namespace.

The `flux-git-sync` module configures Flux to reconcile the tenant's Kubernetes
resources from their Git repository while impersonating the restricted service account.

```cue
bundle: {
	apiVersion: "v1alpha1"
	name:       "dev-team"
	instances: {
		"dev-team": {
			module: url: "oci://ghcr.io/stefanprodan/modules/flux-tenant"
			namespace: "dev-team-apps"
			values: {
				role: "namespace-admin"
				resourceQuota: {
					kustomizations: 100
					helmreleases:   100
				}
			}
		}
		"dev-team-apps": {
			module: url: "oci://ghcr.io/stefanprodan/modules/flux-git-sync"
			namespace: "dev-team-apps"
			values: {
				git: {
					token: string @timoni(runtime:string:DEVTEAM_TOKEN)
					url:   "https://github.com/org/dev-team-apps"
					ref:   "refs/heads/main"
					path:  "./deploy"
				}
				sync: targetNamespace: namespace
			}
		}
	}
}
```

On-board the tenant with:

```shell
export DEVTEAM_TOKEN=<GH TOKEN>
timoni bundle apply -f dev-team.cue --runtime-from-env
```

### Off-board tenants

Off-board the tenant and remove all their workloads with:

```shell
timoni bundle delete dev-team
```
