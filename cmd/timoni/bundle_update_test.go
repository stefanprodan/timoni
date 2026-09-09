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

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	apiv1 "github.com/stefanprodan/timoni/api/v1alpha1"
	"github.com/stefanprodan/timoni/internal/engine"
	"github.com/stefanprodan/timoni/internal/oci"
)

func Test_BundleUpdate(t *testing.T) {
	g := NewWithT(t)

	modName := rnd("my-mod")
	modURL := fmt.Sprintf("%s/%s", dockerRegistry, modName)

	for _, ver := range []string{"1.0.0", "1.1.0", "2.0.0"} {
		_, err := executeCommand(fmt.Sprintf(
			"mod push testdata/module oci://%s -v %s --resolve-symlinks",
			modURL, ver,
		))
		g.Expect(err).ToNot(HaveOccurred())
	}

	digestOf := func(version string) string {
		digest, err := crane.Digest(fmt.Sprintf("%s:%s", modURL, version))
		g.Expect(err).ToNot(HaveOccurred())
		return digest
	}

	writeBundle := func(content string) string {
		path := filepath.Join(t.TempDir(), "bundle.cue")
		g.Expect(os.WriteFile(path, []byte(content), 0o644)).To(Succeed())
		return path
	}

	readFile := func(path string) string {
		data, err := os.ReadFile(path)
		g.Expect(err).ToNot(HaveOccurred())
		return string(data)
	}

	bundleData := fmt.Sprintf(`
bundle: {
	apiVersion: "v1alpha1"
	name:       "test"
	instances: {
		pinned: {
			module: {
				url:     "oci://%[1]s"
				version: "1.0.0" @timoni(update:semver:1.x)
				digest:  "%[2]s"
			}
			namespace: "test"
			values: priority: 10
		}
		unmarked: {
			module: url:     "oci://%[1]s"
			module: version: "1.0.0"
			namespace: "test"
			values: priority: 10
		}
	}
}

`, modURL, digestOf("1.0.0"))

	t.Run("prints the updates without modifying the files", func(t *testing.T) {
		g := NewWithT(t)
		bundlePath := writeBundle(bundleData)

		output, err := executeCommand(fmt.Sprintf("bundle update -f %s --dry-run", bundlePath))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring(fmt.Sprintf("pinned: oci://%s 1.0.0@%s -> 1.1.0@%s", modURL, digestOf("1.0.0"), digestOf("1.1.0"))))
		g.Expect(output).To(ContainSubstring("1 module reference(s) can be updated"))
		g.Expect(output).To(ContainSubstring("(dry run)"))
		g.Expect(output).To(ContainSubstring("instance unmarked skipped: update policy is none"))
		g.Expect(readFile(bundlePath)).To(Equal(bundleData))
	})

	t.Run("updates the files according to the attributes", func(t *testing.T) {
		g := NewWithT(t)
		bundlePath := writeBundle(bundleData)

		output, err := executeCommand(fmt.Sprintf("bundle update -f %s", bundlePath))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring("updated"))

		updated := readFile(bundlePath)
		g.Expect(updated).To(ContainSubstring(`version: "1.1.0" @timoni(update:semver:1.x)`))
		g.Expect(updated).To(ContainSubstring(fmt.Sprintf(`digest:  "%s"`, digestOf("1.1.0"))))
		g.Expect(updated).To(ContainSubstring(`module: version: "1.0.0"`))

		// The updated bundle is valid and its instances build with the new module version.
		_, err = executeCommand(fmt.Sprintf("bundle build -f %s", bundlePath))
		g.Expect(err).ToNot(HaveOccurred())

		// A second run finds the bundle up to date.
		output, err = executeCommand(fmt.Sprintf("bundle update -f %s", bundlePath))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring("all module references are up to date"))
		g.Expect(readFile(bundlePath)).To(Equal(updated))
	})

	t.Run("vets staged updates before writing", func(t *testing.T) {
		g := NewWithT(t)
		bundlePath := writeBundle(bundleData)

		output, err := executeCommand(fmt.Sprintf("bundle update --vet -f %s", bundlePath))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring("pinned: oci://" + modURL + " 1.0.0@"))
		g.Expect(readFile(bundlePath)).To(ContainSubstring(`version: "1.1.0" @timoni(update:semver:1.x)`))

		output, err = executeCommand(fmt.Sprintf("bundle update --vet -f %s", bundlePath))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring("all module references are up to date"))
	})

	t.Run("vets a dry-run without writing", func(t *testing.T) {
		g := NewWithT(t)
		bundlePath := writeBundle(bundleData)

		output, err := executeCommand(fmt.Sprintf("bundle update --vet --dry-run -f %s", bundlePath))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring("(dry run)"))
		g.Expect(readFile(bundlePath)).To(Equal(bundleData))
	})

	t.Run("does not write when staged vet fails", func(t *testing.T) {
		g := NewWithT(t)
		invalid := strings.Replace(bundleData, "\t\t\tnamespace: \"test\"", "\t\t\tnamespace: \"\"", 1)
		bundlePath := writeBundle(invalid)

		_, err := executeCommand(fmt.Sprintf("bundle update --vet -f %s", bundlePath))
		g.Expect(err).To(MatchError(ContainSubstring("failed to build bundle")))
		g.Expect(readFile(bundlePath)).To(Equal(invalid))
	})

	t.Run("updates the unmarked references to the level", func(t *testing.T) {
		g := NewWithT(t)
		bundlePath := writeBundle(bundleData)

		_, err := executeCommand(fmt.Sprintf("bundle update -f %s --level minor", bundlePath))
		g.Expect(err).ToNot(HaveOccurred())

		updated := readFile(bundlePath)
		g.Expect(updated).To(ContainSubstring(`version: "1.1.0" @timoni(update:semver:1.x)`))
		g.Expect(updated).To(ContainSubstring(`module: version: "1.1.0"`))
	})

	t.Run("updates across major versions with an open constraint", func(t *testing.T) {
		g := NewWithT(t)
		bundlePath := writeBundle(strings.Replace(bundleData, "update:semver:1.x", "update:semver:*", 1))

		_, err := executeCommand(fmt.Sprintf("bundle update -f %s", bundlePath))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(readFile(bundlePath)).To(ContainSubstring(`version: "2.0.0" @timoni(update:semver:*)`))
	})

	t.Run("updates the package files of a CUE module", func(t *testing.T) {
		g := NewWithT(t)
		dir := t.TempDir()
		write := func(name, content string) string {
			path := filepath.Join(dir, name)
			g.Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
			g.Expect(os.WriteFile(path, []byte(content), 0o644)).To(Succeed())
			return path
		}
		write("cue.mod/module.cue", "module: \"example.com/fleet\"\nlanguage: version: \"v0.14.0\"\n")
		appFile := write("apps/app.cue", fmt.Sprintf(`package apps

instances: app: {
	module: url:     "oci://%s"
	module: version: "1.0.0" @timoni(update:semver:1.x)
	namespace: "test"
	values: priority: 10
}
`, modURL))
		entryData := `import "example.com/fleet/apps"

bundle: {
	apiVersion: "v1alpha1"
	name:       "apps"
	instances:  apps.instances
}
`
		entry := write("apps/bundle.cue", entryData)

		output, err := executeCommand(fmt.Sprintf("bundle update -f %s --workdir %s", entry, dir))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring("app: oci://" + modURL + " 1.0.0 -> 1.1.0"))
		g.Expect(readFile(appFile)).To(ContainSubstring(`version: "1.1.0" @timoni(update:semver:1.x)`))
		g.Expect(readFile(entry)).To(Equal(entryData))

		_, err = executeCommand(fmt.Sprintf("bundle build -f %s --workdir %s", entry, dir))
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("does not write when a module cannot be listed", func(t *testing.T) {
		g := NewWithT(t)
		unreachable := strings.Replace(bundleData, "module: version: \"1.0.0\"", "module: version: \"1.0.0\" @timoni(update:semver:*)", 1)
		unreachable = strings.Replace(unreachable, fmt.Sprintf("unmarked: {\n\t\t\tmodule: url:     \"oci://%s\"", modURL), fmt.Sprintf("unmarked: {\n\t\t\tmodule: url:     \"oci://%s-missing\"", modURL), 1)
		bundlePath := writeBundle(unreachable)

		_, err := executeCommand(fmt.Sprintf("bundle update -f %s", bundlePath))
		g.Expect(err).To(MatchError(ContainSubstring("instances unmarked: listing versions of")))
		g.Expect(readFile(bundlePath)).To(Equal(unreachable))
	})

	t.Run("updates an indexed local module in dry-run and apply modes", func(t *testing.T) {
		g := NewWithT(t)
		root := t.TempDir()
		moduleOne := filepath.Join(root, "module-1.0.0")
		moduleTwo := filepath.Join(root, "module-1.1.0")
		for _, module := range []string{moduleOne, moduleTwo} {
			g.Expect(os.MkdirAll(module, 0o755)).To(Succeed())
			g.Expect(os.WriteFile(filepath.Join(module, "marker"), []byte(module), 0o644)).To(Succeed())
		}
		digestOne, err := engine.ModuleSourceDigest(moduleOne)
		g.Expect(err).ToNot(HaveOccurred())
		digestTwo, err := engine.ModuleSourceDigest(moduleTwo)
		g.Expect(err).ToNot(HaveOccurred())
		indexPath := filepath.Join(root, "module-index.cue")
		index := fmt.Sprintf(`modules: {
	"oci://registry.example/test/module": versions: {
		"1.0.0": {source: "file://%s", digest: "%s"}
		"1.1.0": {source: "file://%s", digest: "%s"}
	}
}
`, moduleOne, digestOne, moduleTwo, digestTwo)
		g.Expect(os.WriteFile(indexPath, []byte(index), 0o644)).To(Succeed())
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

		output, err := executeCommand(fmt.Sprintf("bundle update --local-index %s -f %s --dry-run", indexPath, bundlePath))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring("file://" + moduleOne + " -> file://" + moduleTwo))
		g.Expect(output).To(ContainSubstring("1.0.0@" + digestOne + " -> 1.1.0@" + digestTwo))
		g.Expect(readFile(bundlePath)).To(Equal(bundle))

		_, err = executeCommand(fmt.Sprintf("bundle update --local-index %s --vet -f %s", indexPath, bundlePath))
		g.Expect(err).ToNot(HaveOccurred())
		updated := readFile(bundlePath)
		g.Expect(updated).To(ContainSubstring("file://" + moduleTwo))
		g.Expect(updated).To(ContainSubstring(`version: "1.1.0" @timoni(update:semver:*)`))
		g.Expect(updated).To(ContainSubstring(`digest: "` + digestTwo + `"`))
	})

	t.Run("switches a matching OCI module to an indexed source", func(t *testing.T) {
		g := NewWithT(t)
		root := t.TempDir()
		moduleOne := filepath.Join(root, "module-1.0.0")
		moduleTwo := filepath.Join(root, "module-1.1.0")
		for _, module := range []string{moduleOne, moduleTwo} {
			g.Expect(os.MkdirAll(module, 0o755)).To(Succeed())
			g.Expect(os.CopyFS(module, os.DirFS("testdata/module"))).To(Succeed())
			g.Expect(os.MkdirAll(filepath.Join(module, "cue.mod/pkg"), 0o755)).To(Succeed())
			schemas, err := filepath.Abs("../../schemas/timoni.sh")
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(os.Remove(filepath.Join(module, "cue.mod/pkg/timoni.sh"))).To(Succeed())
			g.Expect(os.Symlink(schemas, filepath.Join(module, "cue.mod/pkg/timoni.sh"))).To(Succeed())
		}
		digestOne, err := engine.ModuleSourceDigest(moduleOne)
		g.Expect(err).ToNot(HaveOccurred())
		digestTwo, err := engine.ModuleSourceDigest(moduleTwo)
		g.Expect(err).ToNot(HaveOccurred())
		moduleURL := "oci://registry.example/test/module"
		indexPath := filepath.Join(root, "module-index.cue")
		index := fmt.Sprintf(`modules: {
	"%s": versions: {
		"1.0.0": {source: "file://%s", digest: "%s"}
		"1.1.0": {source: "file://%s", digest: "%s"}
	}
}
`, moduleURL, moduleOne, digestOne, moduleTwo, digestTwo)
		g.Expect(os.WriteFile(indexPath, []byte(index), 0o644)).To(Succeed())
		bundlePath := filepath.Join(root, "bundle.cue")
		bundle := fmt.Sprintf(`bundle: {
	apiVersion: "v1alpha1"
	name: "test"
	instances: app: {
		module: url: "%s"
		module: version: "1.0.0" @timoni(update:semver:*)
		module: digest: "%s"
		namespace: "test"
		values: {}
	}
}
`, moduleURL, digestOne)
		g.Expect(os.WriteFile(bundlePath, []byte(bundle), 0o644)).To(Succeed())

		output, err := executeCommand(fmt.Sprintf("bundle update --local-index %s -f %s --dry-run", indexPath, bundlePath))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring(moduleURL + " -> file://" + moduleTwo))
		g.Expect(readFile(bundlePath)).To(Equal(bundle))

		_, err = executeCommand(fmt.Sprintf("bundle update --local-index %s -f %s", indexPath, bundlePath))
		g.Expect(err).ToNot(HaveOccurred())
		updated := readFile(bundlePath)
		g.Expect(updated).To(ContainSubstring("file://" + moduleTwo))
		g.Expect(updated).To(ContainSubstring(`version: "1.1.0" @timoni(update:semver:*)`))
		g.Expect(updated).To(ContainSubstring(`digest: "` + digestTwo + `"`))

		output, err = executeCommand(fmt.Sprintf("bundle build -f %s", bundlePath))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(output).To(ContainSubstring("kind: ConfigMap"))
		g.Expect(output).To(ContainSubstring("app.kubernetes.io/version: 1.1.0"))

		atomicBundlePath := filepath.Join(root, "atomic-bundle.cue")
		g.Expect(os.WriteFile(atomicBundlePath, []byte(bundle), 0o644)).To(Succeed())
		stdout, _, err := executeCommandWithOutErr(fmt.Sprintf("bundle build --update --local-index %s -f %s", indexPath, atomicBundlePath))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(stdout).To(HavePrefix("---\n"))
		g.Expect(stdout).To(ContainSubstring("app.kubernetes.io/version: 1.1.0"))
		g.Expect(readFile(atomicBundlePath)).To(ContainSubstring("file://" + moduleTwo))

		invalidBundlePath := filepath.Join(root, "invalid-bundle.cue")
		invalidBundle := strings.Replace(bundle, "values: {}", "values: priority: \"invalid\"", 1)
		g.Expect(os.WriteFile(invalidBundlePath, []byte(invalidBundle), 0o644)).To(Succeed())
		stdout, _, err = executeCommandWithOutErr(fmt.Sprintf("bundle build --update --local-index %s -f %s", indexPath, invalidBundlePath))
		g.Expect(err).To(HaveOccurred())
		g.Expect(stdout).To(BeEmpty())
		g.Expect(readFile(invalidBundlePath)).To(Equal(invalidBundle))
	})
}

func TestWriteBundleFilesRejectsConcurrentChange(t *testing.T) {
	g := NewWithT(t)
	file := filepath.Join(t.TempDir(), "bundle.cue")
	original := []byte("before")
	g.Expect(os.WriteFile(file, []byte("changed"), 0o644)).To(Succeed())

	err := writeBundleFiles([]string{file}, map[string][]byte{file: original}, map[string][]byte{file: []byte("after")})
	g.Expect(err).To(MatchError(ContainSubstring("changed on disk")))
	data, err := os.ReadFile(file)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(data).To(Equal([]byte("changed")))
}

func TestRunBundleVetUsesStagedOverrides(t *testing.T) {
	g := NewWithT(t)
	bundlePath := filepath.Join(t.TempDir(), "bundle.cue")
	original := `bundle: {
	apiVersion: "v1alpha1"
	name: "test"
	instances: test: {
		module: {
			url: "oci://docker.io/test"
			version: "latest"
		}
		namespace: ""
		values: {}
	}
}
`
	staged := strings.Replace(original, `namespace: ""`, `namespace: "default"`, 1)
	g.Expect(os.WriteFile(bundlePath, []byte(original), 0o644)).To(Succeed())

	previousBundleArgs := bundleArgs
	previousVetArgs := bundleVetArgs
	t.Cleanup(func() {
		bundleArgs = previousBundleArgs
		bundleVetArgs = previousVetArgs
	})
	bundleArgs = bundleFlags{}
	bundleVetArgs = bundleVetFlags{}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	err := runBundleVet(cmd, []string{bundlePath}, true, map[string][]byte{bundlePath: []byte(staged)})
	g.Expect(err).ToNot(HaveOccurred())
	data, err := os.ReadFile(bundlePath)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(data).To(Equal([]byte(original)))
}

func Test_BundleUpdateLocalOCI(t *testing.T) {
	g := NewWithT(t)
	module := filepath.Join(t.TempDir(), "module")
	g.Expect(os.CopyFS(module, os.DirFS("testdata/module"))).To(Succeed())
	vendor := filepath.Join(module, "cue.mod/pkg/timoni.sh")
	g.Expect(os.Remove(vendor)).To(Succeed())
	g.Expect(os.MkdirAll(vendor, 0o755)).To(Succeed())
	g.Expect(os.CopyFS(vendor, os.DirFS("../../schemas/timoni.sh"))).To(Succeed())
	build, err := oci.BuildModuleImage(module, nil, map[string]string{
		apiv1.VersionAnnotation: "1.1.0",
	})
	g.Expect(err).ToNot(HaveOccurred())
	archive := filepath.Join(t.TempDir(), "module.oci.tar")
	g.Expect(oci.WriteImage(build.Image, archive, oci.FormatArchive, []string{"1.1.0"})).To(Succeed())
	digest := build.Digest.String()
	g.Expect(build.Close()).To(Succeed())

	repository := "oci://registry.example/team/module"
	bundlePath := filepath.Join(t.TempDir(), "bundle.cue")
	bundle := fmt.Sprintf(`bundle: {
	apiVersion: "v1alpha1"
	name: "test"
	instances: app: {
		module: url: "%s"
		module: version: "1.0.0" @timoni(update:semver:*)
		module: digest: "sha256:%s"
		namespace: "test"
		values: priority: 10
	}
}
`, repository, strings.Repeat("0", 64))
	g.Expect(os.WriteFile(bundlePath, []byte(bundle), 0o644)).To(Succeed())
	readBundle := func() string {
		data, err := os.ReadFile(bundlePath)
		g.Expect(err).ToNot(HaveOccurred())
		return string(data)
	}
	mapping := repository + "=" + archive

	output, err := executeCommand(fmt.Sprintf("bundle update --oci %s -f %s --dry-run", mapping, bundlePath))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(output).To(ContainSubstring("source " + repository + " -> file://" + archive))
	g.Expect(output).To(ContainSubstring("1.0.0@sha256:" + strings.Repeat("0", 64) + " -> 1.1.0@" + digest))
	g.Expect(readBundle()).To(Equal(bundle))

	_, err = executeCommand(fmt.Sprintf("bundle update --oci %s --vet -f %s", mapping, bundlePath))
	g.Expect(err).ToNot(HaveOccurred())
	updated := readBundle()
	g.Expect(updated).To(ContainSubstring("url: \"file://" + archive + "\""))
	g.Expect(updated).To(ContainSubstring(`version: "1.1.0" @timoni(update:semver:*)`))
	g.Expect(updated).To(ContainSubstring(`digest: "` + digest + `"`))

	_, err = executeCommand(fmt.Sprintf("bundle build -f %s", bundlePath))
	g.Expect(err).ToNot(HaveOccurred())

	atomicBundlePath := filepath.Join(t.TempDir(), "bundle.cue")
	g.Expect(os.WriteFile(atomicBundlePath, []byte(bundle), 0o644)).To(Succeed())
	stdout, _, err := executeCommandWithOutErr(fmt.Sprintf("bundle build --update --oci %s -f %s", mapping, atomicBundlePath))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(stdout).To(HavePrefix("---\n"))
	g.Expect(stdout).To(ContainSubstring("app.kubernetes.io/version: 1.1.0"))
	atomicUpdated, err := os.ReadFile(atomicBundlePath)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(atomicUpdated)).To(ContainSubstring("url: \"file://" + archive + "\""))
}

func Test_BundleBuildUpdateMixedLocalSources(t *testing.T) {
	g := NewWithT(t)
	root := t.TempDir()
	module := filepath.Join(root, "module")
	g.Expect(os.CopyFS(module, os.DirFS("testdata/module"))).To(Succeed())
	vendor := filepath.Join(module, "cue.mod/pkg/timoni.sh")
	g.Expect(os.Remove(vendor)).To(Succeed())
	g.Expect(os.MkdirAll(vendor, 0o755)).To(Succeed())
	g.Expect(os.CopyFS(vendor, os.DirFS("../../schemas/timoni.sh"))).To(Succeed())

	indexedDigest, err := engine.ModuleSourceDigest(module)
	g.Expect(err).ToNot(HaveOccurred())
	build, err := oci.BuildModuleImage(module, nil, map[string]string{
		apiv1.VersionAnnotation: "1.1.0",
	})
	g.Expect(err).ToNot(HaveOccurred())
	archive := filepath.Join(root, "module.oci.tar")
	g.Expect(oci.WriteImage(build.Image, archive, oci.FormatArchive, []string{"1.1.0"})).To(Succeed())
	g.Expect(build.Close()).To(Succeed())

	indexedRepository := "oci://registry.example/indexed/module"
	artifactRepository := "oci://registry.example/artifact/module"
	indexPath := filepath.Join(root, "module-index.cue")
	index := fmt.Sprintf(`modules: {
	"%s": versions: {
		"1.0.0": {source: "file://%s", digest: "%s"}
		"1.1.0": {source: "file://%s", digest: "%s"}
	}
}
`, indexedRepository, module, indexedDigest, module, indexedDigest)
	g.Expect(os.WriteFile(indexPath, []byte(index), 0o644)).To(Succeed())

	bundlePath := filepath.Join(root, "bundle.cue")
	bundle := fmt.Sprintf(`bundle: {
	apiVersion: "v1alpha1"
	name: "test"
	instances: {
		indexed: {
			module: url: "file://%s"
			module: version: "1.0.0" @timoni(update:semver:*)
			module: digest: "%s"
			namespace: "test"
			values: priority: 10
		}
		artifact: {
			module: url: "%s"
			module: version: "1.0.0" @timoni(update:semver:*)
			module: digest: "sha256:%s"
			namespace: "test"
			values: priority: 10
		}
	}
}
`, module, indexedDigest, artifactRepository, strings.Repeat("0", 64))
	g.Expect(os.WriteFile(bundlePath, []byte(bundle), 0o644)).To(Succeed())

	stdout, _, err := executeCommandWithOutErr(fmt.Sprintf(
		"bundle build --update --local-index %s --oci %s=%s -f %s",
		indexPath, artifactRepository, archive, bundlePath,
	))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(stdout).To(HavePrefix("---\n"))
	g.Expect(stdout).To(ContainSubstring("app.kubernetes.io/version: 1.1.0"))

	updated, err := os.ReadFile(bundlePath)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(updated)).To(ContainSubstring("url: \"file://" + module + "\""))
	g.Expect(string(updated)).To(ContainSubstring("url: \"file://" + archive + "\""))
}
