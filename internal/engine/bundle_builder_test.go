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
)

func TestGetBundle(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	t.Run("Get bundle with quoted instance", func(t *testing.T) {
		bundle := `
bundle: {
    apiVersion: "v1alpha1"
    name:       "podinfo"
    instances: {
        "pod-info": {
            module: url:     "oci://ghcr.io/stefanprodan/modules/podinfo"
            module: version: "6.3.5"
            namespace: "podinfo"
            values: caching: {
                enabled:  true
                redisURL: "tcp://redis:6379"
            },
    	 }
         podinfo: {
            module: url:     "oci://ghcr.io/stefanprodan/modules/podinfo"
            module: version: "6.3.5"
            namespace: "podinfo"
            values: caching: {
                enabled:  true
                redisURL: "tcp://redis:6379"
            }
        }
    }
}
`
		v := ctx.CompileString(bundle)
		builder := NewBundleBuilder(ctx, []string{})
		b, err := builder.GetBundle(v)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(b.Instances).To(HaveLen(2))
		g.Expect(b.Instances[0].Name).To(Equal("pod-info"))
		g.Expect(b.Instances[1].Name).To(Equal("podinfo"))
	})
}
