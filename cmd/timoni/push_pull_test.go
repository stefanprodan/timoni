package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func Test_Push_Pull(t *testing.T) {
	modPath := "testdata/cs"

	g := NewWithT(t)
	modURL := fmt.Sprintf("%s/%s", dockerReg, rnd("my-mod", 5))
	modVer := "1.0.0"
	output, err := executeCommand(fmt.Sprintf(
		"push %s oci://%s -v %s",
		modPath,
		modURL,
		modVer,
	))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(output).To(ContainSubstring(modURL))

	tmpDir := t.TempDir()
	output, err = executeCommand(fmt.Sprintf(
		"pull oci://%s -v %s -o %s",
		modURL,
		modVer,
		tmpDir,
	))
	g.Expect(err).NotTo(HaveOccurred())

	// walk the original module and check that all files exist in the pulled module
	fsErr := filepath.Walk(modPath, func(path string, info fs.FileInfo, err error) error {
		if !info.IsDir() {
			tmpPath := filepath.Join(tmpDir, strings.TrimPrefix(path, modPath))
			if _, err := os.Stat(tmpPath); err != nil && os.IsNotExist(err) {
				return fmt.Errorf("file '%s' should exist in pulled module", path)
			}
		}

		return nil
	})
	g.Expect(fsErr).ToNot(HaveOccurred())
}
