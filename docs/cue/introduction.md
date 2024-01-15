# CUE Language

[CUE](https://cuelang.org) is the configuration language used by Timoni. We chose CUE
due to its expressive syntax, powerful data validation model, and ability to generate
Kubernetes configuration while ensuring the result is consistent and unambiguous.

Timoni embeds the CUE engine enabling software vendors to define complex application
deployments packaged as CUE modules. Kubernetes' operators can then refer to these
modules in their own CUE definitions, allowing them to deploy and manage the lifecycle
of applications in a reliable manner.

## What is CUE?

CUE stands for Configure, Unify, Execute. The CUE language is a superset of JSON,
it reduces the verbosity of JSON and allows C-style comments.

While JSON is a data format, CUE is a data constraint language, allowing embedding logic
within the data using if statements, for loops, comprehensions, string interpolation,
arithmetic operations, regular expressions, and more.

CUE can be used for data validation, data templating, configuration, querying,
code generation and even scripting.

## Why CUE for Kubernetes?

What sets CUE apart from other configuration languages is that CUE merges
types, values and constraints into a single concept, think of JSON and JSONSchema all-in-one.

What makes CUE the ideal configuration language for Kubernetes is its ability to
import the Kubernetes OpenAPI schema (both for the builtin Kinds and CRDs)
and use it to validate the generated YAML configuration. When writing templates for Kubernetes
with CUE, you can be sure that the generated YAML is valid and will not be rejected by the API server.

!!! tip "Timoni vendor commands"

    Timoni streamlines the process of importing Kubernetes schemas with the `timoni mod vendor` commands:
    
    - `timoni mod vendor k8s` imports the schema of Kubernetes builtin Kinds
    - `timoni mod vendor crd` imports the schema of Kubernetes CRDs

CUE makes it easy to define complex Kubernetes objects and build abstractions on top of them.
To reduce the boilerplate, CUE allows defining common schemas, constraints and default values
that can be reused across multiple Kubernetes objects.

!!! tip "Timoni CUE definitions"

    Timoni comes with a set of CUE definitions for the most common Kubernetes constructs, such as
    Metadata, Label Selectors, Resource Requirements, Container Images, Image Pull Secrets, and more.

## Who maintains CUE?

The CUE language was originally developed at Google by [Marcel van Lohuizen](https://github.com/mpvl)
who co-created the Borg Configuration Language (BCL). Although CUE is very different from BCL, it
incorporates many of the lessons learned from 15 years of BCL usage.

Nowadays, CUE is an independent open source project maintained by a dedicated team,
with a growing community of contributors and users.

CUE is licensed under the Apache 2.0 license, the source code is hosted on [GitHub](https://github.com/cue-lang/cue)
and accepts contributions from the community, for more information see the
[contributing guidelines](https://github.com/cue-lang/cue/blob/master/CONTRIBUTING.md).

Everyone is welcome to join the CUE community,
for more information please see the [community page](https://cuelang.org/community/).
