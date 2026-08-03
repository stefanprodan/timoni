# Embedding files

Timoni modules can embed plain files (configs, scripts, dashboards, web assets)
using the CUE [`@embed()` attribute](https://cuelang.org/docs/howto/embed-files-in-cue-evaluation/).
At build time, the file contents are loaded into CUE values that templates can use
like any other value, for example to populate the data of a Kubernetes ConfigMap or Secret.

Embedding is the type-safe alternative to inlining file contents as CUE strings:
the files keep their native extension, so they can be edited, linted and highlighted
with the standard tooling for their format, while the module retains full control
over how their contents end up in the generated Kubernetes objects.

## Embedding text files

Assuming you want to ship an `nginx` configuration and a static HTML page with your module,
stored as plain files in the module's `templates` directory:

```text
templates/
├── config.cue
├── configmap.cue
├── nginx.default.conf
└── index.html
```

In the `templates/configmap.cue` file, enable embedding with the file-level
`@extern(embed)` attribute, then embed the sibling files with `@embed()`:

```cue
@extern(embed)

package templates

import (
	timoniv1 "timoni.sh/core/v1alpha1"
)

_nginxConf: string @embed(file="nginx.default.conf", type=text)
_indexHTML: string @embed(file="index.html", type=text)

#ConfigMap: timoniv1.#ImmutableConfig & {
	#config: #Config
	#Kind:   timoniv1.#ConfigMapKind
	#Meta:   #config.metadata
	#Data: {
		"nginx.default.conf": _nginxConf
		"index.html":         _indexHTML
	}
}
```

The file path is relative to the directory of the CUE file containing the attribute.
The `type=text` argument tells CUE to load the file contents as a string; it is required
for file extensions CUE does not recognise, such as `.conf` and `.html`.

Combined with the [`#ImmutableConfig` generator](immutable-config.md), any change to the
embedded files results in a ConfigMap with a new name suffix, which triggers a rolling
update of the workloads referencing it.

## Templating the file contents

Embedded files are static, but their contents can be rendered with the instance
configuration using the CUE `text/template` package, which implements Go templating:

```cue
@extern(embed)

package templates

import (
	"text/template"

	timoniv1 "timoni.sh/core/v1alpha1"
)

_nginxConf: string @embed(file="nginx.default.conf", type=text)

#ConfigMap: timoniv1.#ImmutableConfig & {
	#config: #Config
	#Kind:   timoniv1.#ConfigMapKind
	#Meta:   #config.metadata
	#Data: {
		"nginx.default.conf": template.Execute(_nginxConf, #config)
	}
}
```

With the `templates/nginx.default.conf` file referencing the config values
using Go template expressions:

```nginx
server {
	listen       8080;
	server_name  {{ .metadata.name }};
}
```

For a complete example, see the
[minimal module](https://github.com/stefanprodan/timoni/tree/main/examples/minimal)
which embeds and templates its `nginx` configuration and HTML index page.

## Embedding structured data

When embedding `.json`, `.yaml` or `.toml` files without the `type` argument,
CUE parses the file contents into a structured value instead of a string.
The parsed value can be validated against a CUE schema and further transformed
at build time:

```cue
@extern(embed)

package templates

import "encoding/json"

// Validate the embedded dashboard against a schema.
_dashboard: #GrafanaDashboard @embed(file="dashboard.json")

#DashboardConfigMap: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	// omitted for brevity
	data: "dashboard.json": json.Marshal(_dashboard)
}
```

This goes beyond verbatim file inclusion: invalid files fail the build with a
validation error instead of being applied to the cluster as-is.

## Embedding multiple files

A glob pattern embeds a set of files as a struct, keyed by the file path
relative to the CUE file:

```cue
@extern(embed)

package templates

import "path"

_dashboards: _ @embed(glob="dashboards/*.json", type=text)

#DashboardConfigMap: {
	apiVersion: "v1"
	kind:       "ConfigMap"
	// omitted for brevity
	data: {for p, content in _dashboards {(path.Base(p)): content}}
}
```

Note that the struct keys contain the directory prefix, e.g. `dashboards/latency.json`.
Since Kubernetes does not allow `/` in ConfigMap keys, the comprehension above
uses `path.Base` to strip the directory from the keys.

## Restrictions

CUE enforces the following rules for embedded files:

- Files must be located inside the module's root directory, referencing files
  outside the module with `../` or absolute paths is not allowed.
- Hidden files (dot files) and files under `cue.mod` cannot be embedded,
  nor matched by glob patterns.

When using a `timoni.ignore` file, make sure its patterns do not exclude
embedded files, otherwise `timoni mod push` would strip them from the
module's OCI artifact and the build would fail on the consumer's side.
