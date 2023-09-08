package engine

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	. "github.com/onsi/gomega"
)

var multiline = cmpopts.AcyclicTransformer("multiline", func(s string) []string {
	return strings.Split(s, "\n")
})

func TestCRDYamlToCUE(t *testing.T) {
	ctx := cuecontext.New()
	g := NewWithT(t)

	var oneoff *IntermediateCRD
	filepath.WalkDir(filepath.Join("testdata", "crd"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		t.Run(filepath.Base(path), func(t *testing.T) {
			g := NewWithT(t)
			f, err := os.Open(filepath.Join(path))
			g.Expect(err).ToNot(HaveOccurred())

			b, err := io.ReadAll(f)
			g.Expect(err).ToNot(HaveOccurred())

			cclist, err := YamlCRDToCueIR(ctx, b)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(cclist).ToNot(BeEmpty())
			if filepath.Base(path) == "source-controller.crds.yaml" {
				// this gives us the flux Bucket CRD
				oneoff = cclist[0]
			}

			for i, cc := range cclist {
				// We can't meaningfully check much about converted CRDs in a
				// generic test like this. But a minimal check is still useful -
				// props are populated and schemas contain standard k8s metadata
				// fields at expected paths
				g.Expect(cc.Props.Spec.Names.Kind).ToNot(BeEmpty(), fmt.Sprintf("elem %d", i))
				t.Run(cc.Props.Spec.Names.Kind, func(t *testing.T) {
					NewWithT(t).Expect(cc.Schemas).NotTo(BeEmpty())
					for _, sch := range cc.Schemas {
						isch := sch
						t.Run(sch.Version, func(t *testing.T) {
							g := NewWithT(t)
							apiv := isch.Schema.LookupPath(cue.ParsePath("apiVersion?"))
							g.Expect(apiv.Exists()).To(BeTrue(), fmt.Sprintf("%#v", isch.Schema))
						})
					}
				})
			}
		})

		return nil
	})
	// TODO does gomega have a better string differ? the output is useless when it fails
	n, err := format.Node(oneoff.Schemas[0].Schema.Syntax(cue.All(), cue.Docs(true)))
	g.Expect(err).ToNot(HaveOccurred())

	// hacky one-off test, primarily intended to just let there be _something_
	// committed to disk that shows what converted output looks like.
	//
	// TODO replace with a framework and a test suite that allows testing individual cases of valid constructs within CRD schemas
	diff := cmp.Diff(string(n), goldenBucketFirstSchema, multiline)
	if diff != "" {
		t.Fatal(diff)
	}
}

func TestClosednessHandling(t *testing.T) {
	ctx := cuecontext.New()
	g := NewWithT(t)

	wrapper := `{
	apiVersion: "apiextensions.k8s.io/v1"
	kind:       "CustomResourceDefinition"
	metadata: {
			name: "cases.testing.timoni.sh"
	}
	spec: {
			group: "testing.timoni.sh"
			names: {
					kind:     "Case"
					listKind: "CaseList"
					plural:   "cases"
					singular: "case"
			}
			scope: "Namespaced"
			versions: [{
					name: "v1"
					storage: true
					schema: {
							openAPIV3Schema: {
									type: "object"
									properties: {
										apiVersion: {
												description: "APIVersion defines the versioned schema of this representation of an object. Servers should convert recognized schemas to the latest internal value, and may reject unrecognized values. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources"
												type:        "string"
										}
										kind: {
												description: "Kind is a string value representing the REST resource this object represents. Servers may infer this from the endpoint the client submits requests to. Cannot be updated. In CamelCase. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds"
												type:        "string"
										}
										metadata: {
												type: "object"
										}
										spec: { 
											%s 
										}
									}
							}
					}
			}]
	}
}`

	// put known broken test cases here. Case name as key, a summary of what needs fixing as value.
	//
	// see commented element as an example
	skiplist := map[string]string{
		// "root-unspecified":        "CUE openapi encoder does not know about x-kubernetes-preserve-unknown-fields",
	}

	table := []struct {
		name         string
		spec, status string
		expect       string
	}{
		{
			name: "root-unspecified",
			spec: `type: "object"
			properties: {
				foo: {
					type: "string"
				}
			}
			required: ["foo"]
			`,
			expect: `{
	foo: string
}`,
		},
		{
			name: "root-addlprops-false",
			spec: `type: "object"
			properties: {
				foo: {
					type: "string"
				}
			}
			required: ["foo"]
			additionalProperties: false`,
			expect: `{
	foo: string
}`,
		},
		{
			name: "root-xk-preserve",
			spec: `type: "object"
			properties: {
				foo: {
					type: "string"
				}
			}
			"x-kubernetes-preserve-unknown-fields": true
			required: ["foo"]
			`,
			expect: `{
	foo: string
	...
}`,
		},
		{
			name: "openness-propagation",
			spec: `type: "object"
			properties: {
				foo: {
					type: "string"
				}
				nest: {
					type: "object"
					properties: {
						innerField: type: "string"
						nestnest: {
							type: "object"
							properties: {
								nestnestnest: {
									type: "object"
									properties: {
										innermost: type: "string"
									}
								}
							}
							"x-kubernetes-preserve-unknown-fields": true
						}
					}
				}
			}
			"x-kubernetes-preserve-unknown-fields": true
			required: ["foo", "nest"]
			`,
			expect: `{
	foo: string
	nest: {
		innerField?: string
		nestnest?: {
			nestnestnest?: {
				innermost?: string
			}
			...
		}
	}
	...
}`,
		},
		{
			name: "nested-root-unspecified",
			spec: `type: "object"
			properties: {
				foo: {
					type: "string"
				}
				nest: {
					type: "object"
					properties: {
						innerField: type: "string"
					}
				}
			}
			required: ["foo", "nest"]
			`,
			expect: `{
	foo: string
	nest: {
		innerField?: string
	}
}`,
		},
		{
			name: "nested-root-addlprops-false",
			spec: `type: "object"
			properties: {
				foo: {
					type: "string"
				}
				nest: {
					type: "object"
					properties: {
						innerField: type: "string"
					}
					additionalProperties: false
				}
			}
			required: ["foo", "nest"]
			additionalProperties: false`,
			expect: `{
	foo: string
	nest: {
		innerField?: string
	}
}`,
		},
		{
			name: "nested-root-xk-preserve",
			spec: `type: "object"
			properties: {
				foo: {
					type: "string"
				}
				nest: {
					type: "object"
					properties: {
						innerField: type: "string"
					}
				}
			}
			"x-kubernetes-preserve-unknown-fields": true
			required: ["foo", "nest"]
			`,
			expect: `{
	foo: string
	nest: {
		innerField?: string
	}
	...
}`,
		},
	}

	for _, item := range table {
		tt := item
		t.Run(item.name, func(t *testing.T) {
			if msg, has := skiplist[item.name]; has {
				t.Skip(msg)
			}
			crd := ctx.CompileString(fmt.Sprintf(wrapper, tt.spec))
			g.Expect(crd.Err()).ToNot(HaveOccurred())

			ir, err := convertCRD(crd)
			g.Expect(crd.Err()).ToNot(HaveOccurred())

			g.Expect(err).ToNot(HaveOccurred())

			n := ir.Schemas[0].Schema.LookupPath(cue.ParsePath("#Case.spec")).Syntax(cue.All(), cue.Docs(true))
			// remove the _#def injected by CUE's syntax formatter
			fn, err := format.Node(n.(*ast.StructLit).Elts[1].(*ast.Field).Value)
			g.Expect(err).ToNot(HaveOccurred())
			diff := cmp.Diff(tt.expect, string(fn), multiline)
			if diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

// NOTE - this printed output is created via cue.Value.Syntax() and
// format.Node() on an original cue.Value that was a definition (#-led label).
// In such cases, CUE's formatter wraps the output in _#def: {}, because
// closedness is critical to capture, but is a property of the label,
// rather than the struct value itself.
//
// However, this _#def is ephemerally added only during printing. It does not
// exist at runtime in the graph; do not look for it with e.g. LookupPath().
var goldenBucketFirstSchema = `import "strings"

// Bucket is the Schema for the buckets API
#Bucket: {
	// APIVersion defines the versioned schema of this representation
	// of an object. Servers should convert recognized schemas to the
	// latest internal value, and may reject unrecognized values.
	// More info:
	// https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources
	apiVersion: "source.toolkit.fluxcd.io/v1beta1"

	// Kind is a string value representing the REST resource this
	// object represents. Servers may infer this from the endpoint
	// the client submits requests to. Cannot be updated. In
	// CamelCase. More info:
	// https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	kind: "Bucket"
	metadata: {
		name:      string
		namespace: string
		labels?: {
			[string]: string
		}
		annotations?: {
			[string]: string
		}
	}

	// BucketSpec defines the desired state of an S3 compatible bucket
	spec: #BucketSpec
}

// BucketSpec defines the desired state of an S3 compatible bucket
#BucketSpec: {
	accessFrom?: {
		// NamespaceSelectors is the list of namespace selectors to which
		// this ACL applies. Items in this list are evaluated using a
		// logical OR operation.
		namespaceSelectors: [...{
			// MatchLabels is a map of {key,value} pairs. A single {key,value}
			// in the matchLabels map is equivalent to an element of
			// matchExpressions, whose key field is "key", the operator is
			// "In", and the values array contains only "value". The
			// requirements are ANDed.
			matchLabels?: {
				[string]: string
			}
		}]
	}

	// The bucket name.
	bucketName: string

	// The bucket endpoint address.
	endpoint: string

	// Ignore overrides the set of excluded patterns in the
	// .sourceignore format (which is the same as .gitignore). If not
	// provided, a default will be used, consult the documentation
	// for your version to find out what those are.
	ignore?: string

	// Insecure allows connecting to a non-TLS S3 HTTP endpoint.
	insecure?: bool

	// The interval at which to check for bucket updates.
	interval: string

	// The S3 compatible storage provider name, default ('generic').
	provider?: "generic" | "aws" | "gcp" | *"generic"

	// The bucket region.
	region?: string
	secretRef?: {
		// Name of the referent.
		name: string
	}

	// This flag tells the controller to suspend the reconciliation of
	// this source.
	suspend?: bool

	// The timeout for download operations, defaults to 60s.
	timeout?: string | *"60s"
}

// BucketStatus defines the observed state of a bucket
#BucketStatus: {
	// Artifact represents the output of the last successful Bucket
	// sync.
	artifact?: {
		// Checksum is the SHA256 checksum of the artifact.
		checksum?: string

		// LastUpdateTime is the timestamp corresponding to the last
		// update of this artifact.
		lastUpdateTime?: string

		// Path is the relative file path of this artifact.
		path: string

		// Revision is a human readable identifier traceable in the origin
		// source system. It can be a Git commit SHA, Git tag, a Helm
		// index timestamp, a Helm chart version, etc.
		revision?: string

		// URL is the HTTP address of this artifact.
		url: string
	}

	// Conditions holds the conditions for the Bucket.
	conditions?: [...{
		// lastTransitionTime is the last time the condition transitioned
		// from one status to another. This should be when the underlying
		// condition changed. If that is not known, then using the time
		// when the API field changed is acceptable.
		lastTransitionTime: string

		// message is a human readable message indicating details about
		// the transition. This may be an empty string.
		message: strings.MaxRunes(32768)

		// observedGeneration represents the .metadata.generation that the
		// condition was set based upon. For instance, if
		// .metadata.generation is currently 12, but the
		// .status.conditions[x].observedGeneration is 9, the condition
		// is out of date with respect to the current state of the
		// instance.
		observedGeneration?: >=0 & int

		// reason contains a programmatic identifier indicating the reason
		// for the condition's last transition. Producers of specific
		// condition types may define expected values and meanings for
		// this field, and whether the values are considered a guaranteed
		// API. The value should be a CamelCase string. This field may
		// not be empty.
		reason: strings.MaxRunes(1024) & strings.MinRunes(1) & {
			=~"^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$"
		}

		// status of the condition, one of True, False, Unknown.
		status: "True" | "False" | "Unknown"

		// type of condition in CamelCase or in foo.example.com/CamelCase.
		// --- Many .condition.type values are consistent across
		// resources like Available, but because arbitrary conditions can
		// be useful (see .node.status.conditions), the ability to
		// deconflict is important. The regex it matches is
		// (dns1123SubdomainFmt/)?(qualifiedNameFmt)
		type: strings.MaxRunes(316) & {
			=~"^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$"
		}
	}]

	// LastHandledReconcileAt holds the value of the most recent
	// reconcile request value, so a change of the annotation value
	// can be detected.
	lastHandledReconcileAt?: string

	// ObservedGeneration is the last observed generation.
	observedGeneration?: int

	// URL is the download link for the artifact output of the last
	// Bucket sync.
	url?: string
} | *{
	observedGeneration: -1
}
`
