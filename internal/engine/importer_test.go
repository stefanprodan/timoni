package engine

import (
	"fmt"
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

func TestConvertCRDWithNoSpec(t *testing.T) {
	ctx := cuecontext.New()
	g := NewWithT(t)

	crds := `{
	apiVersion: "apiextensions.k8s.io/v1"
	kind:       "CustomResourceDefinition"
	metadata: {
			name: "nospeccases.testing.timoni.sh"
	}
	spec: {
			group: "testing.timoni.sh"
			names: {
					kind:     "NoSpecCase"
					listKind: "NoSpecCaseList"
					plural:   "nospeccases"
					singular: "nospeccase"
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
									}
							}
					}
			}]
	}
}`

	crd := ctx.CompileString(crds)
	g.Expect(crd.Err()).ToNot(HaveOccurred())

	ir, err := convertCRD(crd)
	g.Expect(err).ToNot(HaveOccurred())

	specNode := ir.Schemas[0].Schema.LookupPath(cue.ParsePath("#NoSpecCaseSpec"))
	g.Expect(specNode.Exists()).To(BeFalse())

	name := ir.Schemas[0].Schema.LookupPath(cue.ParsePath("#NoSpecCase.metadata!.name!"))
	g.Expect(name.Exists()).To(BeTrue())
	namespace := ir.Schemas[0].Schema.LookupPath(cue.ParsePath("#NoSpecCase.metadata!.namespace!"))
	g.Expect(namespace.Exists()).To(BeTrue())
}

func TestConvertCRD(t *testing.T) {
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
		{
			name: "array-xk-preserve",
			spec: `type: "object"
			properties: {
				resources: {
					properties: claims: {
						items: {
							properties: name: type: "string"
							required: ["name"]
							type: "object"
						}
						type: "array"
						"x-kubernetes-list-map-keys": ["name"]
						"x-kubernetes-list-type": "map"
					}
					type: "object"
				}
				spec: {
					properties: template: {
						properties: values: {
							description:                            "Preserve unknown fields."
							type:                                   "object"
							"x-kubernetes-preserve-unknown-fields": true
						}
						type: "object"
					}
					type: "object"
				}
			}
			`,
			expect: `{
	resources?: {
		claims?: [...{
			name: string
		}]
	}
	spec?: {
		template?: {
			// Preserve unknown fields.
			values?: {
				...
			}
		}
	}
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
			g.Expect(err).ToNot(HaveOccurred())

			specNode := ir.Schemas[0].Schema.LookupPath(cue.ParsePath("#CaseSpec")).Syntax(cue.All(), cue.Docs(true))

			// remove the _#def injected by CUE's syntax formatter
			fn, err := format.Node(specNode.(*ast.StructLit).Elts[1].(*ast.Field).Value)
			g.Expect(err).ToNot(HaveOccurred())

			t.Log(string(fn))

			diff := cmp.Diff(tt.expect, string(fn), multiline)
			if diff != "" {
				t.Fatal(diff)
			}
		})
	}
}
