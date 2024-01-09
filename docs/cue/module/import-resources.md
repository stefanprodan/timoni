# Import Kubernetes Resources from YAML

!!! info "Work in progress"

    This guide is under construction and will be updated soon.

The `cue` CLI can import Kubernetes objects from YAML files and convert them to CUE.

Assuming you have a YAML file named `manifests.yaml` with one or more Kubernetes objects,
run the following command in the module's root directory:

```shell
cue import /path/to/manifests.yaml \
  -o -f templates/manifests.cue -p templates \
  -l 'strings.ToLower(kind)' -l 'metadata.name'
```

The generated CUE definitions will be written to `templates/manifests.cue`, from where you can
modify them to fit with the Timoni's template model.

