/*
Copyright 2026 Stefan Prodan

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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"cuelang.org/go/cue/cuecontext"
	. "github.com/onsi/gomega"
)

func writeIndexedModule(t *testing.T, root, marker string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "cue.mod"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"cue.mod/module.cue": "module: \"example.com/test/module\"\n",
		"timoni.cue":         "timoni: {}\n",
		"values.cue":         "values: {}\n",
		"marker.cue":         "marker: \"" + marker + "\"\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestLocalModuleIndexLister_UpdateAndVerifySource(t *testing.T) {
	g := NewWithT(t)
	root := t.TempDir()
	moduleOne := writeIndexedModule(t, filepath.Join(root, "module-1.0.0"), "one")
	moduleTwo := writeIndexedModule(t, filepath.Join(root, "module-2.0.0"), "two")
	digestOne, err := ModuleSourceDigest(moduleOne)
	g.Expect(err).ToNot(HaveOccurred())
	digestTwo, err := ModuleSourceDigest(moduleTwo)
	g.Expect(err).ToNot(HaveOccurred())
	indexPath := filepath.Join(root, "module-index.cue")
	index := fmt.Sprintf(`modules: {
	"oci://registry.example/test/module": versions: {
		"1.0.0": {source: "file://%s", digest: "%s"}
		"2.0.0": {source: "file://%s", digest: "%s"}
	}
}
`, moduleOne, digestOne, moduleTwo, digestTwo)
	g.Expect(os.WriteFile(indexPath, []byte(index), 0o644)).To(Succeed())

	lister, err := NewLocalModuleIndexLister(indexPath, nil)
	g.Expect(err).ToNot(HaveOccurred())
	identity, ok, err := lister.ResolveIdentity("oci://registry.example/test/module", root)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ok).To(BeTrue())
	g.Expect(identity).To(Equal("oci://registry.example/test/module"))
	source, err := lister.ResolveSource(context.Background(), identity, "2.0.0")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(source).To(Equal("file://" + moduleTwo))
	bundlePath := filepath.Join(root, "bundle.cue")
	bundle := fmt.Sprintf(`bundle: {
	apiVersion: "v1alpha1"
	name: "test"
	instances: app: {
		module: url: "file://%s"
		module: version: "1.0.0" @timoni(update:semver:*)
		module: digest: "%s"
		namespace: "test"
		values: {}
	}
}
`, moduleOne, digestOne)
	g.Expect(os.WriteFile(bundlePath, []byte(bundle), 0o644)).To(Succeed())

	updater := NewBundleUpdater(cuecontext.New(), []string{bundlePath})
	updater.SetLocalIndex(lister)
	g.Expect(updater.Load()).To(Succeed())
	plan, err := updater.Plan(context.Background(), lister)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(plan.Changes).To(HaveLen(1))
	change := plan.Changes[0]
	g.Expect(change.ToVersion).To(Equal("2.0.0"))
	g.Expect(change.ToDigest).To(Equal(digestTwo))
	g.Expect(change.ToSource).To(Equal("file://" + moduleTwo))
	g.Expect(updater.Apply(plan)).To(Succeed())
	formatted, err := updater.Format(bundlePath)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(formatted)).To(ContainSubstring("file://" + moduleTwo))
	g.Expect(string(formatted)).To(ContainSubstring(digestTwo))

	// Reject a changed source before applying any bundle updates.
	g.Expect(os.WriteFile(filepath.Join(moduleTwo, "marker.cue"), []byte("marker: \"changed\"\n"), 0o644)).To(Succeed())
	updater = NewBundleUpdater(cuecontext.New(), []string{bundlePath})
	updater.SetLocalIndex(lister)
	g.Expect(updater.Load()).To(Succeed())
	_, err = updater.Plan(context.Background(), lister)
	g.Expect(err).To(MatchError(ContainSubstring("digest mismatch")))
}

func TestModuleSourceDigest_ChangesWithSymlinkTarget(t *testing.T) {
	g := NewWithT(t)
	root := t.TempDir()
	targetOne := filepath.Join(root, "target-one")
	targetTwo := filepath.Join(root, "target-two")
	g.Expect(os.WriteFile(targetOne, []byte("one"), 0o644)).To(Succeed())
	g.Expect(os.WriteFile(targetTwo, []byte("two"), 0o644)).To(Succeed())
	module := filepath.Join(root, "module")
	g.Expect(os.Mkdir(module, 0o755)).To(Succeed())
	link := filepath.Join(module, "link")
	g.Expect(os.Symlink(targetOne, link)).To(Succeed())
	digestOne, err := ModuleSourceDigest(module)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(os.Remove(link)).To(Succeed())
	g.Expect(os.Symlink(targetTwo, link)).To(Succeed())
	digestTwo, err := ModuleSourceDigest(module)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(digestTwo).ToNot(Equal(digestOne))
}
