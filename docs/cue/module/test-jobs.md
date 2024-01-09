# Run tests with Kubernetes Jobs

Module authors can write end-to-end tests as Kubernetes Jobs that are run by Timoni,
after the app workloads are deployed on a cluster.

After the app workloads are installed and become ready, Timoni will apply the Kubernetes Jobs
and will wait for the created pods to run to completion. On upgrades, Timoni will delete the
previous test pods and will recreate the Jobs for the current module values and version.

!!! tip "Idempotency"

    Test runs are idempotent, if the module version and values don't change,
    Timoni will not create new test pods. Tests are run only when a drift is detected
    in desired state.

## Example

To demonstrate how tests work, we'll add a Kubernetes Job that verifies
that the app is accessible from inside the cluster, by running a curl command
against the app's service.

### Test definition

Add the following CUE definition to the module's `templates` directory:

```cue
package templates

import (
	"encoding/yaml"
	"uuid"

	corev1 "k8s.io/api/core/v1"
	batchv1 "k8s.io/api/batch/v1"
	timoniv1 "timoni.sh/core/v1alpha1"
)

#TestJob: batchv1.#Job & {
	#config:    #Config
	apiVersion: "batch/v1"
	kind:       "Job"
	metadata: timoniv1.#MetaComponent & {
		#Meta:      #config.metadata
		#Component: "test"
	}
	metadata: annotations: timoniv1.Action.Force
	spec: batchv1.#JobSpec & {
		template: corev1.#PodTemplateSpec & {
			let checksum = uuid.SHA1(uuid.ns.DNS, yaml.Marshal(#config))
			metadata: annotations: "timoni.sh/checksum": checksum
			spec: {
				containers: [{
					name:            "curl"
					image:           #config.test.image.reference
					imagePullPolicy: #config.test.image.pullPolicy
					command: [
						"curl",
						"-v",
						"-m",
						"5",
						"\(#config.metadata.name):\(#config.service.port)",
					]
				}]
				restartPolicy: "Never"
			}
		}
		backoffLimit: 1
	}
}

```

We set the [force annotation](apply-behavior.md#force-apply) on the Job's metadata to instruct
Timoni to recreate the Job when it's spec changes. And we compute the checksum of the module's
values that we set as an annotation on the Job's pod template.
This will trigger a Job recreation when the module version or values change.

### Test configuration

To allows users to enable testing, we'll add a `test` section to the module's `#Config` definition:

```cue
#Config: {
	test: {
		enabled: *false | bool
		image: timoniv1.#Image & {
			repository: *"docker.io/curlimages/curl" | string
			tag:        *"latest" | string
			digest:     *"" | string
		}
	}
}

```

In the module's `#Instance` definition we'll add the `#TestJob` to the `tests` list:

```cue
#Instance: {
	config: #Config

	tests: curl: #TestJob & {#config: config}
}
```

And finally we'll configure Timoni to apply the tests objects in the `timoni.cue` file with:

```cue
timoni: {

	apply: app: [for obj in instance.objects {obj}]
	
	// Conditionally run tests after an install or upgrade.
	if instance.config.test.enabled {
		apply: test: [for obj in instance.tests {obj}]
	}
}

```

Note that the `apply: test` section goes last, after the `apply: app` section. Timoni
executes the `apply` sections in order, so the app workloads will be deployed first and
then the tests will be applied.

### Test execution

To verify that the test configuration works, enable testing in the module's `debug_values.cue` file:

```cue
values: {
	test: enabled: true
}

```

Running the vet command with the debug should print the test Job:

=== "command"

    ```shell
    timoni mod vet --debug --name myapp
    ```

=== "output"

    ```text
    INF vetting with debug values
    INF Deployment/default/myapp valid resource
    INF Service/default/myapp valid resource
    INF Job/default/myapp-test valid resource
    INF docker.io/curlimages/curl:latest valid image (digest missing)
    INF docker.io/nginx:1-alpine-slim valid image (digest missing)
    INF timoni.sh/myapp valid module
    ```

Running the apply command with the debug values should create the test Job,
after the app workloads are deployed.
