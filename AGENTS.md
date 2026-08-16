# AGENTS.md

High-signal onboarding for AI coding agents working in Timoni.

## What Timoni is

Timoni is a package manager for Kubernetes powered by [CUE](https://cuelang.org/), inspired by Helm.
Apps are defined as **Modules** (type-safe CUE templates + config schema, distributed as OCI artifacts),
deployed as **Instances** (a module rendered with values and applied to a cluster), and composed via
**Bundles** (multiple instances declared together). Helm analogy: module ≈ chart, bundle ≈ umbrella chart,
instance ≈ release. A **Runtime** is the environment a bundle is applied into: it defines the target clusters
and the values fetched live from cluster resources (Secrets/ConfigMaps) for injection into bundles.

## Common commands

```shell
make build              # Build ./bin/timoni (CGO_ENABLED=0)
make test               # tidy + generate + fmt + vet + run all Go tests with Kubernetes envtest
make generate           # Regenerate api/v1alpha1/zz_generated.deepcopy.go via controller-gen
make cue-vet            # cue fmt + cue vet schemas, and `timoni mod vet` the example/blueprint/testdata modules
make docgen            # Regenerate docs/cmd/*.mdx from the cobra commands via `timoni docgen`
```

Run a single test:

```shell
go test ./cmd/timoni/... -run TestApply -v
```

- `make test` is the canonical pre-commit gate; it runs `go mod tidy`, code generation, fmt, and vet before testing, so a clean `make test` implies all of those pass.
- Tests in `cmd/timoni` and packages that talk to the API server require **envtest** (`make install-envtest` downloads the Kubernetes test binaries into `./bin`).
- After changing CUE schemas or example modules, run `make cue-vet`. The blueprint, example and testdata modules vendor the canonical `schemas/timoni.sh/core/v1alpha1` package as relative symlinks under `cue.mod/pkg`, so schema changes propagate without a sync step. After changing types in `api/v1alpha1`, run `make generate`.
- After changing any command's `Use`/`Short`/`Long`/flags in `cmd/timoni/`, run `make docgen` — the CLI reference under `docs/cmd/` is generated and must be regenerated to match, or it goes stale.

## Architecture

The data flow for an `apply` (install/upgrade) is: **fetch module → build CUE → render K8s objects → reconcile onto cluster → record inventory**. Trace it through these layers:

### `cmd/timoni/` — CLI (cobra)
One file per command (e.g. `apply.go`, `build.go`, `bundle_apply.go`, `mod_push.go`), each with a sibling `_test.go`. `main.go` defines the root command, global flags (`--timeout`, `--namespace` via `kubeconfigArgs`, registry flags), and logger injection. Commands wire together the engine, reconciler, oci, and runtime packages — they hold little logic themselves. Tests inject a logger and drive the real cobra command, asserting with the gomega matchers and in-process OCI registry from `internal/testutils`. The CRD vendoring suite is the exception that uses on-disk golden fixtures — regen with the `timoni mod vendor crd …` commands documented at the top of `mod_vendor_crd_test.go`.

### `internal/engine/` — CUE acquisition and compilation
The core of Timoni. Turns CUE into Kubernetes objects:
- `ModuleBuilder` — compiles a module's CUE package into K8s objects, injecting instance name, namespace, module version, and `kubeVersion` (overridable via `TIMONI_KUBE_VERSION`). Default Kubernetes version is the `defaultKubeVersion` constant in `module_builder.go`.
- `ValuesBuilder` — merges `--values` overlays on top of the module's `values.cue`.
- `BundleBuilder` / `RuntimeBuilder` — compile Bundle and Runtime CUE definitions; bundles instantiate per-workspace with runtime values injected.
- `RuntimeInjector` — substitutes runtime values (cluster-read secrets/config) into bundles.
- `ResourceSet` — the rendered set of objects.
- `HealthCheck` — extracts the custom health checks a module declares under `timoni: healthChecks:` (the `#HealthCheck`/`#HealthCheckForCondition` CUE schemas) for custom resources that are not kstatus-compliant.
- `Importer` (`importer.go`) — generates CUE definitions from Kubernetes **CRDs** by converting their OpenAPI v3 schemas (this is what `timoni mod vendor crd` runs, letting module authors use custom resources type-safely).
- `fetcher/` — pulls module sources, either `local.go` (filesystem path) or `oci.go` (OCI registry).

### `internal/reconciler/` — server-side apply
`Reconciler` (built via `NewReconciler`) takes the engine's build result and applies it using `github.com/fluxcd/pkg/ssa` (server-side apply): diff, apply, wait-for-ready, prune stale objects, handle force-recreate of immutable fields. `interactive.go` wraps it as `InteractiveReconciler` — the variant `apply`/`bundle apply` actually use — which adds the `--dry-run` (server-side dry-run) and `--diff` flows, rendering diffs via `internal/dyff`. Honors the `action.timoni.sh/*` annotations (`force`, `prune`, `one-off`, `wait`), all defined in `api/v1alpha1/actions.go`.

### `internal/runtime/` — cluster-side state
Reflects instances on the cluster: the instance **inventory** is stored in a Secret named `timoni.<instance>` (`storage.go`); `instances.go`/`resources.go` read back applied resources; `reader.go` reads runtime values from cluster objects; `job_wait.go` waits on Jobs; `resource_wait.go` plugs the module-defined custom health checks into the kstatus polling as a status reader.

### Runtime & multi-cluster
A **Runtime** (`#Runtime` CUE schema; `apiv1.Runtime`/`RuntimeCluster`) declares a list of target clusters — each with a `name`, `group`, and `kubeContext` — plus `values` read live from the cluster. Bundle commands take it via the persistent `--runtime`/`-r` flag (or `--runtime-from-env`) and select clusters with `--runtime-cluster`/`--runtime-group` (both default `*` = all).

- **Multi-cluster delivery (the key mechanic):** `Runtime.SelectClusters()` resolves the selection, then `bundle apply`/`bundle build` **loop over each selected cluster**, switching `kubeconfigArgs.Context` to that cluster's kube-context per iteration. Each cluster's `name`/`group` are exposed to the bundle as `TIMONI_CLUSTER_NAME`/`TIMONI_CLUSTER_GROUP`, so config can vary per cluster.
- **Two value sources:** live cluster resources (read via `internal/runtime`'s `NewResourceReader` before the build), and — with `--runtime-from-env` — the process environment (`engine.GetEnv()` copies all OS env vars into the runtime values map). The env path is what makes Timoni CI-friendly: a pipeline injects secrets/config as env vars (no cluster read needed) and the bundle references them like any other runtime value.
- **No runtime supplied:** a single `_default` cluster using the current kube-context (`DefaultRuntime`).

### `internal/oci/` — OCI artifact distribution
Push/pull/list/tag/sign modules and generic artifacts to/from container registries via `go-containerregistry` (crane). Signing (`sign_cosign.go`) shells out to an external `cosign` binary that must be on `PATH` — it is not an embedded Go library. Modules are versioned OCI artifacts living next to app images.

### `api/v1alpha1/` — Go API types
Go structs for Bundle, Instance, Runtime, Module, Artifact, Inventory, plus the `action.timoni.sh` / selector annotation constants. `zz_generated.deepcopy.go` is generated — never edit by hand; run `make generate`. `schema.go` bridges to the embedded CUE schemas.

### `schemas/` — CUE schemas (single source of truth)
`schemas/timoni.sh/core/v1alpha1/*.cue` are the canonical CUE definitions (Bundle, Instance, Runtime, etc.), embedded into the binary via `schemas/embed.go` (`//go:embed`) **and** published as the importable `timoni.sh/core/v1alpha1` CUE package that module authors import. Changing a schema here changes both the Go-side validation and what users import.

### `docs/` — user-facing documentation (published to timoni.sh)
The MDX site built with Mintlify (`docs/docs.json` holds the theme, navigation and redirects; preview with `cd docs && npx mint dev`). Pages use Mintlify components (`<Tip>`, `<Tabs>`, `<Card>`), root-relative links without extensions (`/bundle`, `/cue/module/signing`) and `title`/`description` frontmatter. Three tiers, all hand-written except `cmd/`:
- **`docs/` root — the feature/concept guides** (one `.mdx` per feature, e.g. `bundle*.mdx`, `concepts.mdx`, `module.mdx`, plus Flux/GitOps integration pages). Most-missed when behavior changes. The Bundle/Runtime feature set lives here; note `bundle-runtime.mdx` documents the non-obvious `@timoni(runtime:…)` attributes and env-var values. `ls docs/*.mdx` for the full set.
- **`docs/cue/module/`** — the module-authoring behavior contracts that mirror code (apply/prune/wait semantics, immutability, signing, CRD vendoring, test jobs, semver). `ls docs/cue/module/` for the full set.
- **`docs/cmd/`** — the generated CLI reference (do not hand-edit — produced by `make docgen`, gitignored, published from the `website` branch by the docs workflow). New commands must also be added to the CLI Reference tab in `docs/docs.json`.

New pages must be added to the navigation in `docs/docs.json` or they will not appear in the sidebar. The site is published from the `website` branch, which `.github/workflows/docs.yaml` rebuilds from `docs/` plus the generated `cmd/` pages on release tags; never edit that branch by hand.

When you change behavior — flags, apply/prune/wait semantics, the `action.timoni.sh/*` annotations, the Runtime/Bundle schema, vendoring, signing — update the matching page(s) under `docs/` in the same change, not as a follow-up. A Runtime or Bundle change almost always touches a `docs/` root guide *and* a schema; a module-rendering change touches `docs/cue/module/`.

### Other dirs
- `examples/` — runnable modules (redis, etc.) used as docs and as `make cue-vet` / `make push-redis` targets.
- `blueprints/` — module scaffolding templates.
- `actions/` — GitHub Actions wrappers.
- `internal/dyff/` — structured YAML diffing for the interactive flow.
- `internal/testutils/` — gomega matchers and an in-process OCI registry for tests.

## Conventions

- Every Go source file starts with the Apache 2.0 license header (see `api/license.go.txt`).
- Instance, namespace, and runtime names are restricted to lowercase (RFC 1123).
- New code and tests should follow the patterns in existing siblings; commands almost always come with an envtest-backed `_test.go` using the `internal/testutils` helpers.
- Code changes should be accompanied by a corresponding change to the docs.
- Add Go doc comments for new functions and types.
- After modifying a function or type, update its doc comment.
- Add in-line comments for complex logic but don't comment obvious code.
