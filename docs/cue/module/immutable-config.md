# Immutable ConfigMaps and Secrets

Timoni offers a CUE definition `#ImmutableConfig` for generating immutable Kubernetes ConfigMaps and Secrets.

When the ConfigMap or Secret data changes, Timoni will create a new object with a new name suffix,
and it will update the references to the new object, triggering a rolling update for the
application's Deployments, StatefulSets, DaemonSets, etc.
The old ConfigMaps and Secrets will be deleted from the cluster after the rolling update is completed.

## Example

Assuming you want to populate the app Deployment environment variables from a Kubernetes Secret,
with data that end-users can set at installation and upgrade time.

### Create the `Secret` template

In the `templates` directory, create a `secret.cue` file with the following content:

```cue
package templates

import (
	timoniv1 "timoni.sh/core/v1alpha1"
)

#Secret: timoniv1.#ImmutableConfig & {
	#config: #Config
	#Kind:   timoniv1.#SecretKind
	#Meta:   #config.metadata
	#Data: {
		"LOGGING_LEVEL_ROOT": #config.logLevel
	}
}

```

The `#ImmutableConfig` definition will generate an immutable `Secret` resource with the
`metadata.name` set to`<instance-name>-<data-hash>`, where `<data-hash>` is a hash
of the `#Data` object. This ensures that the `Secret` name will change when the
`#Data` content changes.

!!! tip "ConfigMap generator"

    If you want to generate a Kubernetes ConfigMap instead of a Secret,
    set the `#Kind` to `timoniv1.#ConfigMapKind`. 

    If you want to generate multiple ConfigMaps and Secrets, to avoid name collisions,
    set the `#Suffix` to a unique string, e.g. `#Suffix: "-cm1"`.

### Reference the `Secret` in the `Deployment` template

In the `templates/deployment.cue` file, define the `secretName` as an input parameter,
and reference it in `envFrom`:

```cue
#Deployment: appsv1.#Deployment & {
	#config:     #Config
	#secretName: string 

	spec: {
		template: {
			spec: {
				containers: [{
					envFrom: [{
						secretRef: {
							name: #secretName
						}
					}]
				}]
			}
		}
	}
}

```

We need to pass the `secretName` to the `Deployment` template so that every time the
`Secret` name changes, the `Deployment` spec will be updated with the new name.

### Add the `logLevel` to the `Config` definition

In the `templates/config.cue` file, add the `logLevel` configuration:

```cue
#Config: {
    logLevel: *"INFO" | "DEBUG" | "WARN" | "ERROR"
}
```

### Add the `Secret` to the `Instance` definition

In the `templates/config.cue` file, add the `Secret` resource to the instance objects,
and pass the generated `secret.metadata.name` to the `Deployment` template:

```cue
#Instance: {
	config: #Config

	objects: {
		secret: #Secret & {#config: config}

		deploy: #Deployment & {
			#config:     config
			#secretName: secret.metadata.name
		}
	}
}

```
