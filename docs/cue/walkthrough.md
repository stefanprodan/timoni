# CUE Features Walkthrough

To work on Timoni modules a basic understanding of CUE is required. This guide will walk you through
the core features of CUE and how to use them to generate Kubernetes objects.

We'll start with a basic Kubernetes Service definition, and we'll gradually add more features
to reduce the boilerplate and improve the validation of the generated YAML.

## Command Line Tool

Before we begin, make sure you have the CUE CLI installed. To install CUE with Homebrew, run:

```bash
brew install cue
```

For more installation options, follow the instructions from the
[official documentation](https://cuelang.org/docs/install/).

It is recommended to use the same CUE version as the one embedded in Timoni,
which can be found by running: `timoni version`.

!!! tip "Timoni CUE dependency"

    Note that Timoni embeds the CUE engine, so you don't need to install
    it separately in order to use Timoni. The CUE CLI is only required
    when developing modules to format the CUE files before publishing the
    modules to container registries.

CUE comes with a rich set of CLI commands. Throughout this guide, we'll be using the following commands:

- `cue fmt` - format CUE files
- `cue eval` - evaluate CUE expressions
- `cue vet` - validate CUE definitions


## Builtin Types

CUE defines the following type hierarchy:

- `null`
- `bool`
- `string`
- `bytes`
- `number` (`int` and `float`)
- `struct`
- `list`
- `_` (any type)
- `_|_` (error type)

## Structs and Fields

Struct is the most important composite type in CUE, its members are called fields.
A field is a key-value pair, where the key is a string and the value is any CUE type.

We'll use a Kubernetes Service as an example to demonstrate how to define a struct in CUE.

=== "service.cue"

    ```cue
    package main

    nginxSvc: {
        apiVersion: "v1"
        kind:       "Service"
        metadata: {
            name:      "nginx"
            namespace: "default"
        }
        spec: {
            selector: "app.kubernetes.io/name": "nginx"
            ports: [{
                name:       "http"
                port:       80
                targetPort: 80
            }]
        }
    }
    
    ```

=== "service.yaml"

    ```yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: nginx
      namespace: default
    spec:
      selector:
        app.kubernetes.io/name: nginx
      ports:
        - name: http
          port: 80
          targetPort: 80
    ```

To generate the Kubernetes Service YAML, save the CUE definition in a file called `service.cue`,
and used the `cue eval` command to evaluate the `nginxSvc` struct and output it in YAML format:

```shell
cue eval -e nginxSvc --out yaml
```

## Field Immutability

In CUE structs are merged, which means you can define a struct with the same name in multiple
places in the same package, as long as the fields are not duplicated.

For example, you can add a label to the `selector` in a new code block:

=== "service.cue"

    ```cue
    package main

    nginxSvc: {
        apiVersion: "v1"
        kind:       "Service"
        metadata: {
            name:      "nginx"
            namespace: "default"
        }
        spec: {
            selector: "app.kubernetes.io/name": "nginx"
            ports: [{
                name:       "http"
                port:       80
                targetPort: 80
            }]
        }
    }
    
    nginxSvc: spec: selector: "app.kubernetes.io/component": "proxy"
 
    ```

=== "service.yaml"

    ```yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: nginx
      namespace: default
    spec:
      selector:
        app.kubernetes.io/name: nginx
        app.kubernetes.io/component: proxy
      ports:
        - name: http
          port: 80
          targetPort: 80
    ```

In CUE fields are immutable, which means that once a field is set to a concrete value,
its value cannot be changed.

To demonstrate immutability, let's try to rename our service by adding a new code block
to the `service.cue` file:

```cue
nginxSvc: metadata: name: "nginx-2"
```

If you run the eval command, you'll notice that the `name` field reports an error:

```console
$ cue eval -e nginxSvc --out yaml
nginxSvc.metadata.name: conflicting values "nginx-2" and "nginx":
    ./service.cue:5:14
    ./service.cue:18:27
```

## Schema Definitions

CUE definitions, indicated by an identifier starting with `#`,
are used to define schema against which concrete values such as structs can be validated.

The following example demonstrates how to define a basic `#Service` schema
and how to assign it to the `nginxSvc` struct:

=== "schema.cue"

    ```cue
    package main

    #Service: {
        apiVersion: string
        kind:       string
        metadata: {
            name:      string
            namespace: string
        }
        spec: {
            selector: [string]: string
            ports: [{
                name:       string
                port:       int
                targetPort: int | string
            }]
        }
    }
    ```

=== "service.cue"

    ```cue
    package main

    // Set the schema
    nginxSvc: #Service

    // Set the concrete values
    nginxSvc: {
        apiVersion: "v1"
        kind:       "Service"
        metadata: {
            name:      "nginx"
            namespace: "default"
        }
        spec: {
            selector: "app.kubernetes.io/name": "nginx"
            ports: [{
                name:       "http"
                port:       80
                targetPort: 80
            }]
        }
    }
    ```

=== "service.yaml"

    ```yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: nginx
      namespace: default
    spec:
      selector:
        app.kubernetes.io/name: nginx
      ports:
        - name: http
          port: 80
          targetPort: 80
    ```

Save the schema in a file called `schema.cue`. To make our service use the `#Service` schema,
in `service.cue` we'll have to change its definition to `nginxSvc: #Service`.

To validate the `nginxSvc` struct against the `#Service` schema, run:

```shell
cue vet --concrete
```

While the current schema is very basic, it has reduced by a lot the mistakes that can be made
when defining a Kubernetes Service.

- We can no longer define a Service without specifying all the fields present in the schema.
- We can't set a field value to a different type than the one defined in the schema.
- We can't add fields that are not present in the schema, a typo in a field name will not go unnoticed.

To demonstrate the schema validation, let's try to change the `port` field in `service.cue`
to a string e.g. `port: "80"`.

If you run the vet or eval command, you'll notice that the `port` field reports an error:

```console
$ cue eval -e nginxSvc --out yaml
nginxSvc.spec.ports.0.port: conflicting values int and "80" (mismatched types int and string):
    ./schema.cue:14:16
    ./service.cue:17:16
```

CUE allows setting multiple types for a field, in the example above,
the `targetPort` field can be either an `int` or a `string`.
To demonstrate this, let's change the `targetPort` field in `service.cue` to a string,
e.g. `targetPort: "http"`.

If you run the eval command, you'll notice that the `targetPort` field passes
validation and the output YAML contains the string value.

## Default values

In CUE, you can set default values for fields using the `*` operator,
e.g. `apiVersion: string | *"v1"`.

To reduce the boilerplate, we can define default values for fields
such as `apiVersion`, `kind`  and `namespace`.

=== "schema.cue"

    ```cue
    package main
    
    #Service: {
        apiVersion: string | *"v1"
        kind:       string | *"Service"
        metadata: {
            name:      string
            namespace: string | *"default"
        }
        spec: {
            selector: [string]: string
            ports: [{
                name:       string
                port:       int
                targetPort: int | string
            }]
        }
    }
    ```

=== "service.cue"

    ```cue
    package main

    // Set the schema
    nginxSvc: #Service

    nginxSvc: {
        metadata: name: "nginx"
        spec: {
            selector: "app.kubernetes.io/name": "nginx"
            ports: [{
                name:       "http"
                port:       80
                targetPort: "http"
            }]
        }
    }
    ```

=== "service.yaml"

    ```yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: nginx
      namespace: default
    spec:
      selector:
        app.kubernetes.io/name: nginx
      ports:
        - name: http
          port: 80
          targetPort: http
    ```

The fields with default values set in schema can be omitted from the struct definition.

If you run the eval command to generate the YAML,
you'll notice that the fields with defaults are present in output.

## Required Fields

To improve the validation of the `#Service` schema, we can mark fields such as `name` and `port`
as required using the `!` operator e.g. `name!: string`.

=== "schema.cue"

    ```cue
    package main
    
    #Service: {
        apiVersion: string | *"v1"
        kind:       string | *"Service"
        metadata: {
            name!:     string
            namespace: string | *"default"
        }
        spec!: {
            selector: [string]: string
            ports: [{
                name:      string
                port!:      int
                targetPort: int | string
            }]
        }
    }
    ```

=== "service.cue"

    ```cue
    package main

    // Set the schema
    nginxSvc: #Service

    nginxSvc: {
        metadata: {
            name: "nginx"
        }
        spec: {
            selector: "app.kubernetes.io/name": "nginx"
            ports: [{
                name:       "http"
                port:       80
                targetPort: "http"
            }]
        }
    }
    ```

=== "service.yaml"

    ```yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: nginx
      namespace: default
    spec:
      selector:
        app.kubernetes.io/name: nginx
      ports:
        - name: http
          port: 80
          targetPort: http
    ```

To demonstrate the required fields validation, let's try to remove the `port` field from `service.cue`.

If you run the vet or eval command, you'll notice that the `port` field reports an error:

```console
$ cue eval -e nginxSvc --out yaml
nginxSvc.spec.ports.0.port: field is required but not present:
    ./schema.cue:14:4
    ./service.cue:4:11
```

## Optional Fields

To make the `#Service` schema match the Kubernetes specification,
we can mark fields such as `selector` and `ports` as optional, using the `?` operator
e.g. `selector?: [string]: string`.

We'll also add `labels` and `annotations` as optional fields to the `metadata` struct.
And finally, we'll extend the Service spec with `type`, `clusterIP`, `externalName` and `protocol`
as optional fields to complete the schema.

=== "schema.cue"

    ```cue
    package main
    
    #Service: {
        apiVersion: string | *"v1"
        kind:       string | *"Service"
        metadata: {
            name!:     string
            namespace: string | *"default"
		    labels?: [string]:      string
		    annotations?: [string]: string
        }
        spec!: {
            type?:         string
            clusterIP?:    string
            externalName?: string
            selector?: [string]: string
            ports?: [...{
                name?:       string
                protocol:    *"TCP" | "UDP" | "SCTP"
                port!:       int & >=1 & <=65535
                targetPort?: int | string
            }]
        }
    }
    ```

=== "service.cue"

    ```cue
    package main
    
    // Set the schema
    nginxSvc: #Service
    
    nginxSvc: {
        metadata: {
            name:      "nginx"
            namespace: "default"
        }
        spec: {
            type: "ClusterIP"
            selector: "app.kubernetes.io/name": "nginx"
            ports: [{
                name: "http"
                port: 80
            }]
        }
    }
    ```

=== "service.yaml"

    ```yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: nginx
      namespace: default
    spec:
      selector:
        app.kubernetes.io/name: nginx
      ports:
        - name: http
          protocol: TCP
          port: 80
    ```

To demonstrate the optional fields validation, let's try to remove the `targetPort` field from `service.cue`.

If you run the eval command to generate the YAML,
you'll notice that the `targetPort` field is not present in the output.

## Field Constraints

To improve the validation of the `#Service` schema, we can add constraints to its fields.

For example, we can constrain the `name` field to match the Kubernetes naming convention,
using a regular expression e.g. `name!: =~"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"`.

We can also constrain the `port` field to be in the range 1-65535,
using the `>=` and `<=` operators e.g. `port!: >=1 & <=65535`.

We can also constrain the `type` field to match one of the allowed values, using the `|` operator
e.g. `type: *"ClusterIP" | "NodePort" | "LoadBalancer" | "ExternalName"`.

=== "schema.cue"

    ```cue
    package main
    
    #Service: {
        apiVersion: string | *"v1"
        kind:       string | *"Service"
        metadata: {
            name!:     string & =~"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
            namespace: string | *"default"
		    labels?: [string]:      string
		    annotations?: [string]: string
        }
        spec!: {
            type:          *"ClusterIP" | "NodePort" | "LoadBalancer" | "ExternalName"
            clusterIP?:    string
            externalName?: string
            selector?: [string]: string
            ports?: [...{
                name?:       string
                protocol:    *"TCP" | "UDP" | "SCTP"
                port!:       int & >=1 & <=65535
                targetPort?: int | string
            }]
        }
    }
    ```

=== "service.cue"

    ```cue
    package main

    // Set the schema
    nginxSvc: #Service

    nginxSvc: {
        metadata: name: "nginx"
        spec: {
            selector: "app.kubernetes.io/name": "nginx"
            ports: [{
                name:       "http"
                port:       80
                targetPort: "http"
            }]
        }
    }
    ```

=== "service.yaml"

    ```yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: nginx
      namespace: default
    spec:
      selector:
        app.kubernetes.io/name: nginx
      ports:
        - name: http
          protocol: TCP
          port: 80
          targetPort: http
    ```

To demonstrate the field constraints validation, let's try to change the `port` field in `service.cue`
to a value outside the range 1-65535, e.g. `port: 65536` and let's add a disallowed character to the `name`
field, e.g. `name: "nginx_proxy"`.

If you run the vet or eval command, you'll notice both the `name` and `port` fields report an error:

```console
$ cue eval -e nginxSvc --out yaml
nginxSvc.metadata.name: invalid value "nginx_proxy" (out of bound =~"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"):
    ./schema.cue:7:14
    ./service.cue:5:9
nginxSvc.spec.ports.0.port: invalid value 65536 (out of bound <=65535):
    ./schema.cue:20:29
    ./service.cue:16:10
```

## Conditional Fields

In CUE, you can use if statements to add fields to a schema definition conditionally.

For example, we can add the `externalName` field to the `#Service` schema only when the `type`
is set to `ExternalName`.

=== "schema.cue"

    ```cue
    package main
    
    #Service: {
        apiVersion: string | *"v1"
        kind:       string | *"Service"
        metadata: {
            name!:     string & =~"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
            namespace: string | *"default"
        }
        spec!: {
            type: *"ClusterIP" | "NodePort" | "LoadBalancer" | "ExternalName"
            if type == "ExternalName" {
                externalName!: string
            }
            clusterIP?:   string
            selector?: [string]: string
            ports?: [...{
                name?:       string
                protocol:    *"TCP" | "UDP" | "SCTP"
                port!:       int & >=1 & <=65535
                targetPort?: int | string
            }]
        }
    }
    ```

=== "service.cue"

    ```cue
    package main

    // Set the schema
    nginxSvc: #Service

    nginxSvc: {
        metadata: name: "nginx"
        spec: {
            selector: "app.kubernetes.io/name": "nginx"
            ports: [{
                name:       "http"
                port:       80
                targetPort: "http"
            }]
        }
    }
    ```

=== "service.yaml"

    ```yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: nginx
      namespace: default
    spec:
      type: ClusterIP
      selector:
        app.kubernetes.io/name: nginx
      ports:
        - name: http
          protocol: TCP
          port: 80
          targetPort: http
    ```

To demonstrate the condition field constraint, let's add `externalName: "example.com"`
in `service.cue` without setting the `type` to `ExternalName`.

If you run the vet or eval command, you'll notice that the `externalName` field reports an error:

```console  
$ cue eval -e nginxSvc --out yaml
nginxSvc.spec.externalName: field not allowed:
    ./schema.cue:10:9
    ./service.cue:10:3
```

## Field References

In CUE, you can reference field values using dot notation paths.
For example, to reference the `name` field from the `metadata` struct,
you can use the path `metadata.name` anywhere in `spec`.

=== "service.cue"
    
    ```cue
    package main
    
    nginxSvc: #Service & {
        metadata: {
            name: "nginx"
            // Reference the metadata name field
            namespace: name
        }
        spec: {
            // Reference the metadata name field
            selector: "app.kubernetes.io/name": metadata.name
            ports: [{
                name: "http"
                port: 80
                // Reference the port name field
                targetPort: name
            }]
        }
    }
    ```

=== "service.yaml"

    ```yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: nginx
      namespace: nginx
    spec:
      type: ClusterIP
      selector:
        app.kubernetes.io/name: nginx
      ports:
        - name: http
          protocol: TCP
          port: 80
          targetPort: http
    ```

Note that CUE references a value from the nearest enclosing scope, demonstrated above by
referencing the `name` field in both `metadata` and `ports`.
Inside the `metadata` struct, the `name` field references the `metadata.name` value,
while inside the `ports` list, the `name` field references the `ports[0].name` value.

## Aliases

In CUE, an alias defines a local value that is not a member of a struct and is omitted from the output.
Aliases are useful when you want to perform intermediate calculations and reuse the result in multiple places
within the same struct.

For example, we can define an alias for the app name, and use it to set the `name` and `namespace`
fields in `metadata`, and the `app.kubernetes.io/name` label in `selector`.

=== "service.cue"
    
    ```cue
    package main
    
    nginxSvc: #Service & {
        let appName = "nginx"
        metadata: {
            name:      appName
            namespace: appName
        }
        spec: {
            selector: "app.kubernetes.io/name": appName
            ports: [{
                name: "http"
                port: 80
            }]
        }
    }
    
    ```

=== "service.yaml"

    ```yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: nginx
      namespace: nginx
    spec:
      type: ClusterIP
      selector:
        app.kubernetes.io/name: nginx
      ports:
        - name: http
          protocol: TCP
          port: 80
    ```

## Interpolation

Cue supports interpolation in strings, bytes and field names with `\(expr)`.
The expression can be any CUE expression, including references to other fields.

For example, we can declare an alias named `kubeLabel` and use it to interpolate
the domain name in the `selector` labels.
We can also use interpolation to add the `metadata.name` as a prefix to the port `name`.

=== "service.cue"

    ```cue
    package main
    
    nginxSvc: #Service & {
        let kubeLabel = "app.kubernetes.io"
        metadata: {
            name:      "nginx"
            namespace: name
        }
        spec: {
            selector: {
                "\(kubeLabel)/name":      metadata.name
                "\(kubeLabel)/component": "proxy"
            }
            ports: [{
                name: "http-\(metadata.name)"
                port: 80
            }]
        }
    }
    
    ```

=== "service.yaml"

    ```yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: nginx
      namespace: nginx
    spec:
      type: ClusterIP
      selector:
        app.kubernetes.io/name: nginx
        app.kubernetes.io/component: proxy
      ports:
        - name: http-nginx
          protocol: TCP
          port: 80
    ```

Setting prefixes and suffixes to field values can also be accomplished
using the `+` operator, e.g.:

- `name: (metadata.name) + "-http"`
- `name: "http-" + (metadata.name)`

## List Comprehensions

Similar to Python and other languages, CUE supports list comprehensions using the
`[for key, value in list { result }]` syntax.

For example, we can generate a list of Service ports from a list of port numbers.

=== "service.cue"
    
    ```cue
    package main
    
    nginxSvc: #Service & {
        let appPorts = [80, 443]
        metadata: name: "nginx"
        spec: {
            selector: "app.kubernetes.io/name": "nginx"
            ports: [for i, p in appPorts {
                name: "http-\(i)"
                port: p
            }]
        }
    }
    
    ```

=== "service.yaml"
    
    ```yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: nginx
      namespace: default
    spec:
      type: ClusterIP
      selector:
        app.kubernetes.io/name: nginx
      ports:
        - name: http-0
          protocol: TCP
          port: 80
        - name: http-1
          protocol: TCP
          port: 443
    ```

Comprehensions can also be used with conditionals, for example to generate the list of ports, but only
for port numbers in the range 80-443: `[for i, p in appPorts if p >= 80 & p <= 443 { result }]`.

## Embedding

Similar to OOP composition, CUE allows the embedding of a definition into another. Embedding is useful
when you want to create specialised schemas which further constrain the fields of the base schema.

For example, we can embed the `#Service` schema into a `#HeadlessService` schema,
and set concrete values to the `type` and `clusterIP` fields.

=== "schema.cue"
    
    ```cue
    package main
    
    #HeadlessService: #Service & {
        spec!: {
            type:      "ClusterIP"
            clusterIP: "None"
        }
    }
    
    #Service: {
        apiVersion: string | *"v1"
        kind:       string | *"Service"
        metadata: {
            name!:     string & =~"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
            namespace: string | *"default"
            labels?: [string]:      string
            annotations?: [string]: string
        }
        spec!: {
            type:         *"ClusterIP" | "NodePort" | "LoadBalancer" | "ExternalName"
            appProtocol?: string
            clusterIP?:   string
            if type == "ExternalName" {
                externalName!: string
            }
            selector?: [string]: string
            ports?: [...{
                name?:       string
                protocol:    *"TCP" | "UDP" | "SCTP"
                port!:       int & >=1 & <=65535
                targetPort?: int | string
            }]
        }
    }
    
    ```

=== "service.cue"
    
    ```cue
    package main
    
    nginxSvc: #HeadlessService & {
        metadata: name: "nginx"
        spec: {
            selector: "app.kubernetes.io/name": "nginx"
            ports: [{
                name:       "http"
                port:       80
            }]
        }
    }
    ```

=== "service.yaml"
    
    ```yaml
    apiVersion: v1
    kind: Service
    metadata:
      name: nginx
      namespace: default
    spec:
      type: ClusterIP
      clusterIP: None
      selector:
        app.kubernetes.io/name: nginx
      ports:
        - name: http
          protocol: TCP
          port: 80
    ```

To make our service use the `#HeadlessService` schema, we'll have to change its definition
to `nginxSvc: #HeadlessService`.

While we can configure the metadata and ports, we can no longer
set the `type` and `clusterIP` fields, as they are set by the `#HeadlessService` schema.
