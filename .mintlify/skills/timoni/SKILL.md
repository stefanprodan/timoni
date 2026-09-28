---
name: timoni
description: Use when deploying applications to Kubernetes with Timoni. Covers installing and upgrading module instances from OCI registries, composing multi-app deployments with bundles, injecting values from clusters or CI with runtimes, targeting multiple clusters, and authoring, testing, signing and publishing modules with CUE.
license: Apache-2.0
metadata:
  author: Stefan Prodan
  homepage: https://timoni.sh
  source: https://github.com/stefanprodan/timoni
  version: "0.3.0"
---

# Timoni

Timoni is a package manager for Kubernetes powered by CUE and inspired by Helm.
An app is packaged as a **module** (CUE templates plus a typed config schema)
and distributed as an OCI artifact next to the app container images.
A module deployed to a cluster is an **instance**. A **bundle** declares a set of
instances deployed together, and a **runtime** declares the target clusters and
the values read from them at apply time.

Helm mapping: chart = module, umbrella chart = bundle, release = instance.

Full documentation index: https://timoni.sh/llms.txt

## When to use

- Install, upgrade, diff or uninstall an app on Kubernetes from an OCI module.
- Deploy several apps as one unit with a `bundle.cue` file.
- Vary a bundle per environment or cluster, reading values from Secrets,
  ConfigMaps, custom resources or CI environment variables.
- Deploy the same bundle to a fleet of clusters in one command.
- Author a module: define the config schema, template Kubernetes objects,
  vendor CRD schemas, add test jobs and health checks, sign and publish it.

Do not manage resources owned by a Timoni instance with `helm`, `kustomize` or
`kubectl apply`. Timoni owns them through server-side apply and prunes what is
no longer rendered.

With Flux, Timoni is a templating engine only (`timoni build` / `bundle build`
output pushed as manifests): lifecycle management, runtime value injection,
bundling and multi-cluster delivery do not apply once Flux takes over.

## Quick reference

Every CLI command supports `--help` and prints usage examples along with the
available flags. Run `timoni <command> --help` for details beyond the tables
below.

### Instances

| Task | Command |
|------|---------|
| Install or upgrade | `timoni -n <ns> apply <name> oci://<repo> -v <version> -f values.cue` |
| Preview as a server-side diff | `timoni -n <ns> apply <name> oci://<repo> -v <version> --diff` |
| Recreate objects with immutable-field changes | `... apply ... --force` |
| Take over an instance owned by a bundle | `... apply ... --overwrite-ownership` |
| Uninstall | `timoni -n <ns> delete <name>` |
| List instances | `timoni list -A` |
| Status and readiness | `timoni -n <ns> status <name>` |
| Show module or managed resources | `timoni -n <ns> inspect module\|resources <name>` |
| Show stored values | `timoni -n <ns> inspect values <name>` |
| Render manifests, no cluster needed | `timoni build <name> oci://<repo> -v <version> -f values.cue [--mask-secrets]` |

### Bundles and runtimes

| Task | Command |
|------|---------|
| Validate | `timoni bundle vet -f bundle.cue` |
| Validate without a cluster | `timoni bundle vet -f bundle.cue -r runtime.cue --offline` |
| Validate and print the computed bundle | `timoni bundle vet -f bundle.cue --print-value` |
| Preview | `timoni bundle apply -f bundle.cue --diff` |
| Apply | `timoni bundle apply -f bundle.cue [-f bundle_secrets.cue]` |
| Render to files | `timoni bundle build -f bundle.cue --output-dir ./manifests` |
| Render to stdout without Secret values | `timoni bundle build -f bundle.cue --mask-secrets` |
| Status | `timoni bundle status -f bundle.cue` or `timoni bundle status <name>` |
| Delete | `timoni bundle delete -f bundle.cue` or `timoni bundle delete <name>` |
| Update module versions per policy | `timoni bundle update -f bundle.cue [--level patch\|minor\|major]` |
| Preview module updates | `timoni bundle update -f bundle.cue --dry-run` (prints `old -> new` per instance, writes nothing) |
| With a runtime | add `-r runtime.cue`; select with `--runtime-cluster <name>` / `--runtime-group <group>` |
| Runtime values from CI env vars | add `--runtime-from-env` |
| Print resolved runtime values | `timoni runtime build -f runtime.cue [--cluster <name>] [--cluster-group <group>]` |

### Modules and registries

| Task | Command |
|------|---------|
| Log in to a registry | `echo $TOKEN \| timoni registry login ghcr.io -u <user> --password-stdin` |
| List published versions | `timoni mod list oci://<repo>` (newest 100; `--limit 0` for all, `--with-digest=false` to skip digests) |
| Pull a module to disk | `timoni mod pull oci://<repo> -v <version> -o ./module` |
| Show the README | `timoni mod show readme oci://<repo> -v <version>` (or a local `./module` path) |
| Show the config schema | `timoni mod show config oci://<repo> -v <version>` (or a local `./module` path) |
| Verify signature on pull | `... mod pull ... --verify=cosign --cosign-key=cosign.pub`, or keyless: `--verify=cosign --certificate-identity-regexp=<re> --certificate-oidc-issuer=<url>` |
| Create a module | `timoni mod init <name> --blueprint oci://ghcr.io/stefanprodan/timoni/blueprints/starter` |
| Validate a module | `timoni mod vet [path] [--debug]` |
| Vendor Kubernetes schemas | `timoni mod vendor k8s [-v 1.30]` |
| Vendor CRD schemas | `timoni mod vendor crd -f <crds.yaml or URL> [--kind Kind,Kind.group] [-v v1] [--prune]` (`--list` prints the kinds and versions without vendoring) |
| Publish | `timoni mod push ./module oci://<repo> -v <semver> [--latest=false] [--sign=cosign [--cosign-key=cosign.key]]` |
| Build an OCI archive without a registry | `timoni mod build ./module -v <semver> -o module.oci.tar` |
| Generic artifacts | `timoni artifact push oci://<repo> -t <tag> -f ./dir`, `timoni artifact pull oci://<repo>:<tag>`, `timoni artifact build -f ./dir -t <tag> -o out.oci.tar` |
| Format CUE | `timoni fmt` |

Global flags: `-n/--namespace`, `--kube-context`, `--kubeconfig`, `--timeout`.
Registry commands read `~/.docker/config.json`; `--creds user:token` is a
per-command flag that exposes the token in shell history and process lists,
so prefer `timoni registry login --password-stdin`.

## Values

Values are supplied with `-f/--values` as CUE, YAML or JSON files, merged in
order, or from stdin with `-f -`. A CUE values file wraps everything in `values:`:

```cue
values: {
	replicas: 2
	image: tag: "1.27-alpine"
	resources: limits: memory: "128Mi"
}
```

Values are validated against the module's `#Config`; a type or constraint
mismatch fails the build before anything reaches the cluster. To discover the
available values, run `timoni mod show config oci://<repo> -v <version>` (or
`timoni mod show config ./module` for a local module).

## Bundles

```cue
bundle: {
	apiVersion: "v1alpha1"
	name:       "podinfo"
	instances: {
		redis: {
			module: {
				url:     "oci://ghcr.io/stefanprodan/modules/redis"
				version: "8.10.1" @timoni(update:semver:8.x)
			}
			namespace: "podinfo"
			values: maxmemory: 256
		}
		podinfo: {
			module: {
				url:     "oci://ghcr.io/stefanprodan/modules/podinfo"
				version: "6.14.0" @timoni(update:semver:6.x)
			}
			namespace: "podinfo"
			values: caching: {
				enabled:  true
				redisURL: "tcp://redis:6379"
			}
		}
	}
}
```

- Before adding an instance, read the module docs first:
  `timoni mod show readme oci://<repo> -v <version>`, then
  `timoni mod show config oci://<repo> -v <version>` for the full `#Config`
  schema the `values:` are validated against.
- Editing loop: `timoni fmt bundle.cue` formats the file,
  `timoni bundle vet -f bundle.cue` validates the definition without a
  cluster (add `--print-value` to inspect the computed bundle),
  `timoni bundle build -f bundle.cue` renders the manifests offline, and
  `timoni bundle apply -f bundle.cue --diff` previews the changes against the
  cluster before applying (exits 1 on drift, 2 on failure).
- Instances are applied in declaration order, each waiting for readiness
  before the next (`--wait=false` disables waiting). Deletion runs in reverse
  order.
- `module.version` defaults to `latest`. A semver tag can be overwritten unless
  the registry enforces tag immutability; pin `module.digest` when you need
  deterministic retrieval.
- Local modules in a bundle use `module: url: "file://../modules/app"`,
  relative to the bundle file, or an absolute path as
  `file:///abs/path/to/module`; `version`/`digest` are ignored and the
  instance gets version `0.0.0-devel`.
- Split a bundle across files and merge with repeated `-f` (for example a
  `bundle_secrets.cue` kept out of git or piped from stdin with `-f -`).
  SOPS-encrypted YAML/JSON partials:
  `sops exec-file --filename secrets.yaml bundle.secrets.yaml 'timoni bundle apply -f bundle.cue -f {}'`.
- Instances applied by a bundle are owned by it. `timoni apply` on such an
  instance, or another bundle claiming it, fails unless
  `--overwrite-ownership` is passed.
- Bundles can import CUE packages from `cue.mod` in the working directory
  (`--workdir` selects the CUE module root).
- `timoni bundle update -f bundle.cue` rewrites the module versions and
  digests according to the `@timoni(update:semver:<constraint>|digest|none)`
  attribute on the `version` field; `--level patch|minor|major` updates the
  references without an attribute, and `--dry-run` only prints the changes.
  Run `timoni bundle vet` and `timoni bundle build` afterwards, the update
  does not validate the values against the new module schema.

## Runtimes and multi-cluster

A runtime declares clusters and values to read from them:

```cue
runtime: {
	apiVersion: "v1alpha1"
	name:       "fleet"
	clusters: {
		"staging": {group: "staging",    kubeContext: "eks-staging"}
		"prod-eu": {group: "production", kubeContext: "eks-prod-eu"}
		"prod-us": {group: "production", kubeContext: "eks-prod-us"}
	}
	values: [
		{
			query: "k8s:v1:Secret:infra:redis-auth"
			for: {"REDIS_PASS": "obj.data.password"}
		},
		{
			query:    "k8s:v1:ConfigMap:infra:aws-info"
			for:      {"REGION": "obj.data.region"}
			optional: true
		},
	]
}
```

Bind runtime values in a bundle with `@timoni(runtime:<string|number|bool>:<VAR>)`;
a concrete value next to the attribute is the default when the variable is absent:

```cue
bundle: {
	_pass:   string      @timoni(runtime:string:REDIS_PASS)
	_region: "eu-west-1" @timoni(runtime:string:REGION)
	_env:    string      @timoni(runtime:string:TIMONI_CLUSTER_GROUP)
	...
}
```

With `--runtime-from-env`, accepted by every `timoni bundle` subcommand, the
variables come from the process environment instead of a cluster; no
`-r` runtime file is needed:

```shell
REDIS_PASS=$SECRET timoni bundle apply -f bundle.cue --runtime-from-env
```

- With `-r runtime.cue`, `bundle apply|vet|status|delete` iterate over the
  selected clusters, switching kube-context per cluster. `bundle build`
  requires exactly one selected cluster (`--runtime-cluster`).
- `TIMONI_CLUSTER_NAME` and `TIMONI_CLUSTER_GROUP` are set for clusters
  declared in a runtime, not for the implicit current-context default.
- Value precedence: cluster identity values override cluster query results,
  which override `--runtime-from-env` values. Secret `data` is base64-decoded.
- `optional: true` only tolerates the queried object being absent. A missing
  variable bound to a field without a default still fails with an incomplete
  value error.
- `bundle build -r` reads runtime queries from the selected cluster; only
  bundles without cluster queries build offline.

## Apply semantics

`timoni apply` and `bundle apply`, per instance:

1. Pull the module, merge values, build and validate the objects.
2. Create the namespace if missing.
3. Record the intended inventory (install) or a pending revision (upgrade) in
   Secret `timoni.<instance>`.
4. Server-side apply the `timoni: apply:` sets one after another, in
   declaration order (for example CRDs before workloads, tests last), waiting
   for readiness after each set (kstatus plus module-declared health checks).
5. Prune objects from the previous inventory that are no longer rendered.
6. Commit the final inventory.

Applying over objects created by `kubectl apply` or `helm install` takes
ownership: field ownership moves to Timoni and the `kubectl.kubernetes.io/last-applied-configuration`
and `meta.helm.sh/*` annotations are removed. This is the supported Helm
migration path; make sure a name collision is intentional before applying.

Per-object behavior is set with annotations in the module templates:

| Annotation | Effect |
|------------|--------|
| `action.timoni.sh/force: "enabled"` | Recreate the object when its immutable fields change (`--force` does this for all objects) |
| `action.timoni.sh/one-off: "enabled"` | Apply only if the object does not exist |
| `action.timoni.sh/prune: "disabled"` | Never garbage-collect the object, also survives `delete` |
| `action.timoni.sh/wait: "disabled"` | Skip readiness waiting for the object |

`timoni delete` and `bundle delete` remove the inventory objects (except
`prune: disabled` ones) and wait for finalizers; namespaces are left in place.

## Module authoring

Typical layout produced by the starter blueprint (a convention, not a contract):

```
myapp/
├── cue.mod/
│   ├── gen/           # vendored Kubernetes API and CRD schemas
│   ├── pkg/           # timoni.sh/core/v1alpha1 schemas
│   └── module.cue     # module: "timoni.sh/myapp"
├── templates/
│   ├── config.cue     # #Config schema with defaults, #Instance
│   ├── deployment.cue # #Deployment: {#config: #Config, ...}
│   └── service.cue
├── images.cue         # container images defaults (repository, tag, digest)
├── timoni.cue         # entry point: values, timoni.instance, timoni.apply
├── timoni.ignore      # files excluded from mod push
├── values.cue         # placeholder for user values
├── README.md
└── LICENSE
```

- `#Config` holds defaults (`*value | type`) and constraints (`int & >0`);
  `#Instance` turns a config into `objects`.
- `timoni.cue` exposes `values: templates.#Config`, builds the instance and
  lists the apply sets: `apply: app: [for obj in instance.objects {obj}]`.
  Timoni supplies the instance name and namespace as CUE tags and
  `moduleVersion`/`kubeVersion` as tag variables; `timoni.cue` must bind them
  into the config (the blueprint does this in `timoni: instance: config:`).
- Core helpers in `timoni.sh/core/v1alpha1`: `#Metadata`, `#Selector`,
  `#Image`, `#SemVer`, `#ResourceRequirements`, `#ImmutableConfig`
  (content-hashed ConfigMap/Secret names that trigger rollouts), `#Affinity*`,
  `#SecurityContext*` presets, `#Monitor*`, `#HealthCheckLibrary` and
  `#HealthCheck` for custom resources. Read the imported schemas under
  `cue.mod/pkg` for exact fields.
- Embed plain files (configs, scripts) with `@extern(embed)` on the package
  and `@embed(file=...)` on a field.
- Custom resources: `timoni mod vendor crd -f <crds.yaml>` generates CUE
  definitions under `cue.mod/gen` with the CRD schema embedded. Select the
  kinds and versions with `--kind` (case-insensitive `Kind` or `Kind.group`)
  and `-v`; the command fails when a selector matches nothing. Use `--list`
  to print the kinds and versions in a file, and `--prune` to remove the
  stale definitions of the file's API groups. `timoni mod vet`
  validates the module's custom resources against the vendored CRDs and the
  CRDs rendered by the module with the API server checks (OpenAPI schema,
  list map/set uniqueness, CEL rules). The same checks run in `build`,
  `bundle build` and `bundle apply`, `--validate=false` disables them.
  Add `timoni: healthChecks:` entries for CRs that are not kstatus-compliant.
- Test jobs: emit Jobs in a final `apply: test:` set with the
  `action.timoni.sh/force: "enabled"` annotation and a checksum of the config
  in the pod template; the Job is recreated when that checksum changes, not on
  every apply.
- Dev loop: `timoni fmt` (formats the module recursively, skipping
  `cue.mod`), `timoni mod vet` (uses `debug_values.cue` with `--debug`),
  `timoni -n test build <name> .`, `timoni -n test apply <name> . --diff`.
  `TIMONI_KUBE_VERSION` overrides the Kubernetes version assumed at build.
- Publish with `timoni mod push . oci://<repo> -v <semver>`; `latest` moves
  unless `--latest=false`. Sign with `--sign=cosign` (keyless in CI or with
  `--cosign-key`); consumers verify on `mod pull` with `--verify=cosign` plus
  the key or the certificate identity and OIDC issuer flags. `cosign` must be
  on `PATH`. `timoni mod build` produces an unsigned OCI archive for
  air-gapped transfer; it is not an apply input.

## Output hygiene

`--diff` masks Secret data as `***`; `build` and `bundle build` mask it only
with `--mask-secrets` (stdout only, ignored with `--output-dir`). Masking
covers only the data of Kubernetes Secret objects; secret values a module
places elsewhere (container args, env vars, ConfigMaps) print in plaintext
regardless. Everything else prints plaintext: `bundle vet --print-value`,
`runtime build`, `inspect values`. Keep such output out of shared CI logs.

## Gotchas

- Names: instance, namespace, bundle and cluster names are lowercase
  alphanumerics with `-`, `_` or `.` inside, 63 chars max; Kubernetes object
  names generated from them still follow Kubernetes rules.
- `--dry-run` reports the action per object (created, configured, unchanged)
  without applying; `--diff` does the same and also prints the field changes,
  exiting with code 1 when drift is detected and code 2 when the dry run
  fails, so it doubles as a drift check in CI.
- Objects removed from a module are pruned on the next apply. Guard
  data-bearing objects with `action.timoni.sh/prune: "disabled"`.
- Immutable-field changes (Job template, StatefulSet volume claims, Service
  clusterIP) fail the apply; `--force` or the force annotation deletes and
  recreates the object, which can cause downtime or data loss.
- `apply`/`build` accept an unpacked local module directory or an `oci://`
  URL, not git URLs or local OCI archives.
- `mod vet` needs no registry or cluster; `build` of an OCI module needs the
  registry; `apply` needs both.

## Safe apply workflow

1. Pin the version: `timoni mod list oci://<repo>`; use a digest where tags are mutable.
2. Read the docs: `timoni mod show readme oci://<repo> -v <version>`, then
   the schema: `timoni mod show config oci://<repo> -v <version>`.
3. Validate offline: `timoni build ...` or `timoni bundle vet -f bundle.cue --offline`.
4. Preview: `--diff`, review pruned and recreated objects.
5. Apply, then `timoni status` / `timoni bundle status`.
6. Confirm merged values with `inspect values` only where the output is not logged.

## Resources

### Documentation MCP server

URL: https://timoni.sh/mcp

A streamable HTTP MCP server, no authentication required. It provides tools
for searching the published documentation and fetching pages as markdown.
Prefer its results over prior knowledge when answering Timoni questions.
Register it under the name `timoni-docs`.

### Documentation in markdown format

When the MCP server is not available, fetch the pages below directly; each
URL returns the page as markdown.

- [Quickstart Guide](https://timoni.sh/quickstart.md): Deploy a demo application on Kubernetes using a Timoni module published in a container registry.
- [Concepts](https://timoni.sh/concepts.md): Modules, instances, bundles and artifacts: the building blocks of Timoni.
- [Installation Guide](https://timoni.sh/install.md): Install the Timoni CLI on Linux, macOS and Windows.
- [Timoni compared to other tools](https://timoni.sh/comparison.md): How Timoni compares to Helm, Kustomize and other Kubernetes packaging tools.
- [Bundle](https://timoni.sh/bundle.md): Declare groups of module instances and their values in a single CUE file.
- [Bundle Runtime](https://timoni.sh/bundle-runtime.md): Fetch values at apply time from Kubernetes Secrets, ConfigMaps and other resources.
- [Bundle Update](https://timoni.sh/bundle-update.md): Update the module versions and digests referenced in bundles according to declared update policies.
- [Bundle Distribution](https://timoni.sh/bundle-distribution.md): Publish bundles and runtimes as OCI artifacts to container registries.
- [Bundle Secrets Injection](https://timoni.sh/bundle-secrets.md): Inject secrets into bundles with runtime attributes or SOPS encrypted files.
- [Multi-cluster Deployments](https://timoni.sh/bundle-multi-cluster.md): Deliver applications across clusters and environments with bundles and runtimes.
- [Module Specification](https://timoni.sh/module.md): Structure, configuration schema and metadata of a Timoni module.
- [GitHub Actions](https://timoni.sh/github-actions.md): Build, test and push modules from GitHub workflows.
- [Flux AIO Distribution](https://timoni.sh/flux-aio.md): A lightweight Flux CD distribution packaged as a Timoni module.
- [Helm interoperability with Flux](https://timoni.sh/flux-helm-interop.md): Orchestrate Helm chart deployments from Timoni bundles through Flux.
- [GitOps Guide](https://timoni.sh/gitops-flux.md): Build a GitOps delivery pipeline for module instances with Timoni and Flux.
- [Get Started with Timoni Modules](https://timoni.sh/cue/module/initialization.md): Create a new module and learn its structure and development workflow.
- [Immutable ConfigMaps and Secrets](https://timoni.sh/cue/module/immutable-config.md): Generate immutable ConfigMaps and Secrets that roll out on change.
- [Embedding files](https://timoni.sh/cue/module/embedding-files.md): Embed configs, scripts and other plain files in modules with the @embed attribute.
- [Kubernetes Custom Resources](https://timoni.sh/cue/module/custom-resources.md): Define and validate Kubernetes custom resources in modules.
- [Kubernetes Version Constraints](https://timoni.sh/cue/module/semver-constraints.md): Adapt module output to the Kubernetes version of the target cluster.
- [Control the Apply Behavior](https://timoni.sh/cue/module/apply-behavior.md): Change how resources are applied with the action.timoni.sh annotations.
- [Custom Health Checks](https://timoni.sh/cue/module/health-checks.md): Declare readiness checks for custom resources that don't follow kstatus.
- [Run tests with Kubernetes Jobs](https://timoni.sh/cue/module/test-jobs.md): Write end-to-end tests as Kubernetes Jobs run by Timoni after deployment.
- [Import Kubernetes Resources from YAML](https://timoni.sh/cue/module/import-resources.md): Convert existing Kubernetes YAML manifests to CUE templates.
- [Module Publishing](https://timoni.sh/cue/module/publishing.md): Version and publish modules as OCI artifacts.
- [Module Signing and Verification](https://timoni.sh/cue/module/signing.md): Sign module artifacts with Cosign or Notation and verify them at apply time.
- [Module Distribution with GitHub Actions](https://timoni.sh/cue/module/github-actions.md): Publish module versions from GitHub workflows.
