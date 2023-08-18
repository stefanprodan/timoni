package engine

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
)

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
	// g.Expect(string(n)).To(Equal(goldenBucketFirstSchema))
	diff := cmp.Diff(string(n), goldenBucketFirstSchema)
	if diff != "" {
		t.Fatal(diff)
	}
}

// NOTE - this printed output is created using the %#v format verb to print a a
// cue.Value that points to a CUE definition (#-led label). In such cases, CUE's
// formatter wraps the output in _#def: {}, because closedness is key to
// express, but is not an internal property of a struct itself, but the label to
// which its attached.
//
// However, this _#def is ephemeral, added only for printing - it does not
// actually exist in the runtime graph.
var goldenBucketFirstSchema = `import "strings"

_#def
_#def: {
	// APIVersion defines the versioned schema of this representation
	// of an object. Servers should convert recognized schemas to the
	// latest internal value, and may reject unrecognized values.
	// More info:
	// https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources
	apiVersion?: string

	// Kind is a string value representing the REST resource this
	// object represents. Servers may infer this from the endpoint
	// the client submits requests to. Cannot be updated. In
	// CamelCase. More info:
	// https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	kind?: string
	metadata?: {
		...
	}

	// BucketSpec defines the desired state of an S3 compatible bucket
	spec?: {
		// AccessFrom defines an Access Control List for allowing
		// cross-namespace references to this object.
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
				...
			}]
			...
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

		// The name of the secret containing authentication credentials
		// for the Bucket.
		secretRef?: {
			// Name of the referent.
			name: string
			...
		}

		// This flag tells the controller to suspend the reconciliation of
		// this source.
		suspend?: bool

		// The timeout for download operations, defaults to 60s.
		timeout?: string | *"60s"
		...
	}

	// BucketStatus defines the observed state of a bucket
	status?: {
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
			...
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
			...
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
		...
	} | *{
		observedGeneration: -1
		...
	}
	...
}
`
