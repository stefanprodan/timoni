# Get Started with Timoni Modules

This guide will walk you through the process of creating a new Timoni module.
We'll explore the structure of the created module, and the various development
commands that Timoni provides to aid module development.

Furthermore, this guide provides an overview of the templating system,
including the module configuration values, the Kubernetes templates,
and how to generate Kubernetes objects from these templates.

## Initialize a module

To create a module in the current directory, run the following command:

```shell
timoni mod init myapp \
--blueprint oci://ghcr.io/stefanprodan/timoni/blueprints/starter
```

The starter blueprint will create a simple module that deploys a NGINX web server.

## Module structure

The init command creates a directory named `myapp` with the following structure:

```sh
myapp
├── cue.mod
│   ├── gen # Kubernetes APIs and CRDs schemas
│   ├── pkg # Timoni APIs schemas
│   └── module.cue # Module metadata
├── templates
│   ├── config.cue # Config schema and default values
│   ├── deployment.cue # Kubernetes Deployment template
│   └── service.cue # Kubernetes Service template
├── timoni.cue # Timoni entry point
├── timoni.ignore # Timoni ignore rules
├── values.cue # Timoni values placeholder 
├── LICENSE # Module license
└── README.md # Module documentation
```

Navigate to the root directory of the new module with `cd myapp`,
this is where we'll be working from now on.

## Development commands

Timoni comes with a set of commands that help with module development.

At most times, after making changes, you'll be using the
[`timoni mod vet`](#vetting-the-module) command to verify that
the module config and the Kubernetes templates are valid.

### Build and apply the module

To build a module instance, run the following command:

```shell
timoni -n test build nginx .
```

The build command generates a Kubernetes Deployment and Service,
and prints the Kubernetes resources to stdout in YAML format.

If you inspect the output, you'll notice that the Kubernetes `metadata`
matches the instance name and namespace specified in the build command:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app.kubernetes.io/managed-by: timoni
    app.kubernetes.io/name: nginx
    app.kubernetes.io/version: 0.0.0-devel
  name: nginx
  namespace: test
```

By convention, Timoni uses the `app.kubernetes.io/name` label to set the
Deployment selector and the Service selector.

The `app.kubernetes.io/version` label is used to track the module version
from the container registry, and it's set to `0.0.0-devel` by default, when
building a module locally.

To create the module instance on a Kubernetes cluster:

```shell
timoni -n test apply nginx .
```

The apply command will create the Kubernetes resources and will wait for the
Deployment and Service to become ready. If the `test` namespace doesn't exist,
Timoni will create it.

!!! tip "Diff changes"

    When making changes to the module, you can use the `timoni apply --diff`
    flag to see the differences between the in-cluster resources
    and the newly generated ones.

To delete the module instance:

```shell
timoni -n test delete nginx .
```

The delete command will remove the Kubernetes Deployment and Service from the cluster,
but will leave the `test` namespace intact, because the namespace resource
is not part of the module.

### Build with custom values

For debugging purposes, the blueprint contains a `debug_values.cue` file
that can be used to test the module with custom values.

To build a module instance with the debug values, run the following command:

```shell
timoni -n test build nginx . --values debug_values.cue
```

If you inspect the output, you'll notice that the Deployment container image
has changed from `docker.io/nginx:1-alpine` to `docker.io/nginx:1-alpine-slim`,
as specified in the `debug_values.cue` file.

!!! tip "Ignore rules"

    Note that the `debug_values.cue` file is listed in `timoni.ignore`,
    and will be excluded when publishing the module to a container registry.
    The debug values are meant to be used only for local testing while developing
    the module.

### Vetting the module

The vet command is your best friend when developing modules.
It verifies that the module structure is compliant with the Timoni specification,
then it builds the module and validates the generated Kubernetes
resources against their CUE schemas.

To vet the module with default values, run the following command:

=== "command"

    ```shell
    timoni mod vet --name nginx
    ```

=== "output"

    ```text
    INF vetting with default values
    INF Deployment/default/nginx valid resource
    INF Service/default/nginx valid resource
    INF docker.io/nginx:1-alpine valid image (digest missing)
    INF timoni.sh/myapp valid module
    ```

The vet command will print the list of Kubernetes resources and their validation status,
along with the container images referenced in the module.

It is also possible to verify the module using the debug values:

=== "command"

    ```shell
    timoni mod vet --debug --name nginx --namespace test
    ```

=== "output"

    ```text
    INF vetting with debug values
    INF Deployment/test/nginx valid resource
    INF Service/test/nginx valid resource
    INF docker.io/nginx:1-alpine-slim valid image (digest missing)
    INF timoni.sh/myapp valid module
    ```

If the vet command encounters an invalid definition, it will print the error message,
the file and line number where the error occurred.

### Format the module files

Similar to Go, CUE has a built-in code formatter that can be used to format CUE files.

To format all files in a module, run the following command:

```shell
cue fmt ./...
```

It is recommended to run this command after making changes to a module. Most editors
have a CUE plugin that can run the `cue fmt` command automatically when saving a file.

### Update the Kubernetes schemas

To ensure that the Kubernetes resources defined in a module
are in conformance with their OpenAPI schema, Timoni offers a command for
vendoring CUE definitions generated from the Kubernetes builtin APIs.

To update the schemas to the latest Kubernetes stable release, run the following command:

```shell
timoni mod vendor k8s
```

The vendor command will download the Kubernetes schemas from GitHub
[container registry](https://github.com/stefanprodan/kubernetes-cue-schema/pkgs/container/timoni%2Fkubernetes-schema),
and will update the CUE definitions from the `cue.mod/gen/k8s.io` directory.

### Update the Timoni schemas

Timoni comes with a set of CUE definitions (schemas and generators), that are used to reduce the
boilerplate code when developing modules. These definitions are included in the modules generated
with `timoni mod init`, and are vendored in the `cue.mod/pkg/timoni` directory.

To update the Timoni schemas to the latest version,
run the following command from within the module root:

```shell
timoni artifact pull oci://ghcr.io/stefanprodan/timoni/schemas -o cue.mod/pkg
```

!!! tip "Schemas versioning"

    The schemas are published with every Timoni release, each Timoni version has
    a corresponding schemas artifact tag. While the Timoni API is in alpha,
    the schemas may change between releases in a non-backwards compatible way.

## Templates overview

The templates directory contains the module configuration schema
and the Kubernetes resources templates.

### Config definition

The `config.cue` file contains a [schema definition](../walkthrough.md#schema-definitions)
called `#Config` that is used to specify which input fields can be configured by end-users
when applying a module instance.

For each input field, the `#Config` definition can specify the field type, if it is
required or optional, if it has a [default value](../walkthrough.md#default-values),
and which [validation rules](../walkthrough.md#field-constraints) should be applied.

For example, the `#Config` contains a `replicas` field defined like this:

```cue
#Config: {
	replicas: *1 | int & >0
}

```

This means that the `replicas` field defaults to the value of `1`, and when specified,
the user-supplied value must be an integer greater than zero.

The `replicas` value is used in the `deployment.cue` template to set the
`spec.replicas` field of the Kubernetes Deployment:

```cue
#Deployment: appsv1.#Deployment & {
	#config: #Config
	spec: {
		replicas: #config.replicas
	}
}

```

### Instance definition

The `config.cue` file contains a definition called `#Instance` that is used to specify
the list of Kubernetes objects that will be generated from templates when
building and applying a module instance.

For example, the `#Instance` definition takes as input a `#Config` object
and returns the list of objects with the `#Deployment` and `#Service` types:

```cue
#Instance: {
	config: #Config

	objects: {
		deploy: #Deployment & {#config: config}
		service: #Service & {#config: config}
	}
}

```

The `#Instance` definition is used in by the Timoni entry point,
defined in `timoni.cue` file from the root directory,
which injects the instance name, namespace and user-supplied config
values and then applies the generated objects on the cluster.

```cue
timoni: {
	instance: templates.#Instance & {
		config: values
		config: metadata: {
			name:      string @tag(name)
			namespace: string @tag(namespace)
		}
	}

	apply: app: [for obj in instance.objects {obj}]
}

```

### Kubernetes definitions

The `deployment.cue` and `service.cue` files contain the
[CUE definitions](../walkthrough.md#schema-definitions)
used to generate the Deployment and Service Kubernetes objects.

Let's take a look at the `service.cue` file:

```cue
package templates

import (
	corev1 "k8s.io/api/core/v1"
)

#Service: corev1.#Service & {
	#config:    #Config
	apiVersion: "v1"
	kind:       "Service"
	metadata:   #config.metadata
	if #config.service.annotations != _|_ {
		metadata: annotations: #config.service.annotations
	}
	spec: corev1.#ServiceSpec & {
		selector: #config.selector.labels
		ports: [
			{
				port:       #config.service.port
				protocol:   "TCP"
				name:       "http"
				targetPort: name
			},
		]
	}
}
```

The `package` directive specifies the CUE package name,
the package name should match the directory name.

The `import` directive is used to import packages from the `cue.mod` directory,
in this case the `k8s.io/api/core/v1` package is imported as `corev1`. We need this
package to generate any Kubernetes object that is part of the `v1` API group, like
ConfigMap, Secret, ServiceAccount, Service, etc.

With `#Service: corev1.#Service & {...}` we specify that our `#Service` definition
is a Kubernetes Service object, and that it should inherit all the fields from the
`corev1.#Service` type. This ensures that the generated object will be validated
against the Kubernetes API schema.

Inside the `#Service` definition, we have a `#config` field of type `#Config`.
The `#config` field is used as an input parameter for the user-supplied values.

The rest of the `#Service` definition is used to set the Kubernetes object fields
to the `#config` values.

Optional config fields, like the `service.annotations`, should be set only if the user
supplied a value for them. To verify if a field has a value, we can use an if statement
and map the config field inside:

```cue
if #config.service.annotations != _|_ {
    metadata: annotations: #config.service.annotations
}
```

Besides mapping the config fields to the Kubernetes object fields, we can also
set fixed values, like the port `protocol` and `name` fields.

## Extend the module config

Assuming that you want to allow users to expose the NGINX service outside the cluster
as a NodePort or LoadBalancer type. To do this, you can add a field called `type`
to the `service` section of the `#Config` definition, then map the `type` value to the
Kubernetes Service `spec.type` field.

### Add the config field

Open the `config.cue` file and add the `type` field to the `service` section:

```cue
#Config: {
	service: {
		type: *"ClusterIP" | "NodePort" | "LoadBalancer"
	}
}

```

With the `*` operator we specify that the `type` field has a default value of `ClusterIP`.

With the `|` operator we enumerate the allowed values for the `type` field.

!!! tip "Documentation"

    Note that you should document newly added fields in the module's `README.md` file.
    The readme contains a table with the module configuration fields, their type,
    default value and description.

### Map the field in the template

Open the `service.cue` file and set the `spec.type` field to the `#config.service.type` value:

```cue
#Service: corev1.#Service & {
	#config: #Config

	spec: {
		type: #config.service.type
	}
}

```

### Test the config

To test the new config field, we can build the module with the default values,
and check the generated Service:

```shell
timoni build nginx . | grep ClusterIP
```

To test if we can change the Service type,
we can use the `debug_values.cue` file:

```cue
values: {
	service: type: "NodePort"
}
```

And build the module with the debug values:

```shell
timoni build nginx . -f debug_values.cue | grep NodePort
```

Finally, we can test the validation rules by setting an
invalid value, e.g. `service: type: "foo"`.

Running the build or vet command with the debug values
should print a validation error:

```shell
timoni mod vet --debug
```




