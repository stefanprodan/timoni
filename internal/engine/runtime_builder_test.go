/*
Copyright 2023 Stefan Prodan

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package engine

import (
	"testing"

	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
)

func TestRuntimeBuilder_Minimal(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	rt := `
runtime: {
	apiVersion: "v1alpha1"
	name:       "minimal"
}
`
	v := ctx.CompileString(rt)
	builder := NewRuntimeBuilder(ctx, []string{})
	b, err := builder.GetRuntime(v)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(b.Name).To(BeEquivalentTo("minimal"))
}

func TestRuntimeBuilder_Values(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	rt := `
runtime: {
	apiVersion: "v1alpha1"
	name:       "test"
	values: [
		{
			query: "k8s:v1:ConfigMap:kube-system:kube-root-ca.crt"
			for: {
				"CLUSTER_CA": "obj.data.\"ca.crt\""
			}
			optional: true
		},
		{
			query: "k8s:source.toolkit.fluxcd.io/v1:GitRepository:flux-system:cluster"
			for: {
				"CLUSTER_REVISION": "obj.status.artifact.revision"
				"CLUSTER_STATUS":   "[for c in obj.status.conditions if c.type == \"Ready\" {c.status}][0]"
			}
		},
		{
			query: "k8s:cert-manager.io/v1:ClusterIssuer:letsencrypt"
			for: {
				"CLUSTER_ISSUER": "obj.spec.acme.email"
			}
		},
	]
}
`
	v := ctx.CompileString(rt)
	builder := NewRuntimeBuilder(ctx, []string{})
	b, err := builder.GetRuntime(v)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(b.Name).To(BeEquivalentTo("test"))
	g.Expect(b.Refs[0].Optional).To(BeTrue())
	g.Expect(b.Refs[1]).To(BeEquivalentTo(apiv1.RuntimeResourceRef{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "source.toolkit.fluxcd.io/v1",
			Kind:       "GitRepository",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster",
			Namespace: "flux-system",
		},
		Expressions: map[string]string{
			"CLUSTER_REVISION": "obj.status.artifact.revision",
			"CLUSTER_STATUS":   "[for c in obj.status.conditions if c.type == \"Ready\" {c.status}][0]",
		},
		Optional: false,
	}))
	g.Expect(b.Refs[2].Namespace).To(BeEmpty())
}

func TestRuntimeBuilder_Clusters(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	rt := `
runtime: {
	apiVersion: "v1alpha1"
	name:       "fleet"
	clusters: {
		"staging-eu": {
			group:       "staging"
			kubeContext: "eu-central-1:staging"
		}
		"staging-us": {
			group:       "staging"
			kubeContext: "us-west-1:staging"
		}
		"production-eu": {
			group:       "production"
			kubeContext: "eu-central-1:production"
		}
		"production-us": {
			group:       "production"
			kubeContext: "us-west-1:production"
		}
	}
}
`
	v := ctx.CompileString(rt)
	builder := NewRuntimeBuilder(ctx, []string{})
	b, err := builder.GetRuntime(v)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(b.Name).To(BeEquivalentTo("fleet"))
	g.Expect(len(b.Clusters)).To(BeEquivalentTo(4))
	g.Expect(b.Clusters[0]).To(BeEquivalentTo(apiv1.RuntimeCluster{
		Name:        "staging-eu",
		Group:       "staging",
		KubeContext: "eu-central-1:staging",
	}))
	g.Expect(b.Clusters[3]).To(BeEquivalentTo(apiv1.RuntimeCluster{
		Name:        "production-us",
		Group:       "production",
		KubeContext: "us-west-1:production",
	}))
}
