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
	"os"
	"path/filepath"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
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
            module: url:     "file://./modules/podinfo"
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

func TestBundleBuilderWorkspace(t *testing.T) {
	g := NewWithT(t)
	ctx := cuecontext.New()

	srcDir := t.TempDir()
	bundleFile := filepath.Join(srcDir, "bundle.cue")
	bundleData := `
bundle: {
	apiVersion: "v1alpha1"
	name:       "test"
	instances: {
		app: {
			module: url:     "file://./modules/app"
			module: version: "1.0.0"
			namespace: "apps"
			values: message: string @timoni(runtime:string:TEST_WORKSPACE_MSG)
		}
	}
}
`
	g.Expect(os.WriteFile(bundleFile, []byte(bundleData), 0o644)).To(Succeed())

	builder := NewBundleBuilder(ctx, []string{bundleFile})

	t.Run("builds workspace in memory", func(t *testing.T) {
		g := NewWithT(t)
		workspace := "cluster-1"
		err := builder.InitWorkspace(workspace, map[string]string{"TEST_WORKSPACE_MSG": "from-runtime"})
		g.Expect(err).ToNot(HaveOccurred())

		_, err = os.Lstat(builder.WorkspaceDir(workspace))
		g.Expect(err).To(MatchError(os.ErrNotExist))

		v, err := builder.Build(workspace)
		g.Expect(err).ToNot(HaveOccurred())

		msg := v.LookupPath(cue.ParsePath("bundle.instances.app.values.message"))
		g.Expect(msg.Err()).ToNot(HaveOccurred())
		g.Expect(msg).To(WithTransform(func(v cue.Value) string {
			s, _ := v.String()
			return s
		}, Equal("from-runtime")))
	})

	t.Run("resolves relative file URLs against the origin", func(t *testing.T) {
		g := NewWithT(t)
		workspace := "cluster-2"
		err := builder.InitWorkspace(workspace, map[string]string{"TEST_WORKSPACE_MSG": "from-runtime"})
		g.Expect(err).ToNot(HaveOccurred())

		v, err := builder.Build(workspace)
		g.Expect(err).ToNot(HaveOccurred())

		b, err := builder.GetBundle(v)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(b.Name).To(Equal("test"))
		g.Expect(b.Instances).To(HaveLen(1))
		g.Expect(b.Instances[0].Module.Repository).To(Equal("file://" + filepath.Join(srcDir, "modules", "app")))
	})

	t.Run("isolates workspaces per cluster", func(t *testing.T) {
		g := NewWithT(t)
		for _, cluster := range []string{"east", "west"} {
			err := builder.InitWorkspace(cluster, map[string]string{"TEST_WORKSPACE_MSG": cluster})
			g.Expect(err).ToNot(HaveOccurred())
		}
		for _, cluster := range []string{"east", "west"} {
			v, err := builder.Build(cluster)
			g.Expect(err).ToNot(HaveOccurred())
			msg, err := v.LookupPath(cue.ParsePath("bundle.instances.app.values.message")).String()
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(msg).To(Equal(cluster))
		}
	})

	t.Run("keeps a user file named schema.cue", func(t *testing.T) {
		g := NewWithT(t)
		userSchemaFile := filepath.Join(srcDir, "schema.cue")
		userSchemaData := `
bundle: instances: app: values: extra: "from-user-schema-file"
`
		g.Expect(os.WriteFile(userSchemaFile, []byte(userSchemaData), 0o644)).To(Succeed())

		builder := NewBundleBuilder(ctx, []string{bundleFile, userSchemaFile})
		workspace := "cluster-1"
		err := builder.InitWorkspace(workspace, map[string]string{"TEST_WORKSPACE_MSG": "from-runtime"})
		g.Expect(err).ToNot(HaveOccurred())

		v, err := builder.Build(workspace)
		g.Expect(err).ToNot(HaveOccurred())

		extra, err := v.LookupPath(cue.ParsePath("bundle.instances.app.values.extra")).String()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(extra).To(Equal("from-user-schema-file"))
	})

	t.Run("reports error positions relative to the workspace", func(t *testing.T) {
		g := NewWithT(t)
		workspace := "cluster-err"
		err := builder.InitWorkspace(workspace, nil)
		g.Expect(err).ToNot(HaveOccurred())

		_, err = builder.Build(workspace)
		g.Expect(err).To(HaveOccurred())

		details := cueerrors.Details(err, &cueerrors.Config{Cwd: builder.WorkspaceDir(workspace)})
		g.Expect(details).To(ContainSubstring("0.bundle.cue"))
		g.Expect(details).ToNot(ContainSubstring(builder.WorkspaceDir(workspace)))
	})
}
