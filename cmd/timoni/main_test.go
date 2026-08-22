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

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/configuration"
	"github.com/distribution/distribution/v3/registry"
	_ "github.com/distribution/distribution/v3/registry/auth/htpasswd"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/go-logr/zerologr"
	"github.com/mattn/go-shellwords"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
	"github.com/rs/zerolog"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	runtimeLog "sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	envTestClient  client.Client
	dockerRegistry string
)

func TestRootCommandPreservesContextCancellation(t *testing.T) {
	g := NewWithT(t)
	ctx, cancel := context.WithCancel(context.Background())
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)

	rootCmd.PersistentPreRun(cmd, nil)
	cancel()

	g.Expect(cmd.Context().Err()).To(MatchError(context.Canceled))
}

func TestMain(m *testing.M) {
	ctx := ctrl.SetupSignalHandler()
	err := setupRegistryServer(ctx)
	if err != nil {
		panic(fmt.Sprintf("failed to start docker registry: %s", err))
	}

	testEnv := &envtest.Environment{}
	if _, err := testEnv.Start(); err != nil {
		panic(err)
	}

	user, err := testEnv.ControlPlane.AddUser(envtest.User{
		Name:   "envtest-admin",
		Groups: []string{"system:masters"},
	}, nil)
	if err != nil {
		panic(err)
	}

	kubeConfig, err := user.KubeConfig()
	if err != nil {
		panic(err)
	}

	tmpDir, err := os.MkdirTemp("", "timoni")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	tmpFilename := filepath.Join(tmpDir, rnd("kubeconfig"))
	if err := os.WriteFile(tmpFilename, kubeConfig, 0644); err != nil {
		panic(err)
	}

	envTestClient, err = client.New(testEnv.Config, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		panic(fmt.Sprintf("failed to create k8s client: %v", err))
	}

	kubeconfigArgs.KubeConfig = &tmpFilename
	rootArgs.cacheDir = tmpDir

	code := m.Run()
	if err := testEnv.Stop(); err != nil {
		panic(err)
	}
	os.Exit(code)
}

func executeCommand(cmd string) (string, error) {
	return executeCommandWithIn(cmd, nil)
}

func TestRestoreSignalHandlingAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})

	go restoreSignalHandling(ctx, func() { close(stopped) })
	cancel()

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("signal handling was not restored")
	}
}

func executeCommandWithIn(cmd string, in io.Reader) (string, error) {
	buf := new(bytes.Buffer)
	err := executeCommandWithStreams(cmd, in, buf, buf)
	return buf.String(), err
}

// executeCommandWithOutErr runs the command and returns the stdout
// and stderr streams separately, the logger writes to stderr.
func executeCommandWithOutErr(cmd string) (string, string, error) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	err := executeCommandWithStreams(cmd, nil, stdout, stderr)
	return stdout.String(), stderr.String(), err
}

func executeCommandWithStreams(cmd string, in io.Reader, stdout, stderr io.Writer) error {
	defer resetCmdArgs()
	args, err := shellwords.Parse(cmd)
	if err != nil {
		return err
	}

	rootCmd.SetOut(stdout)
	rootCmd.SetErr(stderr)
	rootCmd.SetArgs(args)
	// Always set the input stream so a reader injected by a previous
	// test does not leak into commands that read stdin.
	if in == nil {
		in = os.Stdin
	}
	rootCmd.SetIn(in)

	zcfg := zerolog.ConsoleWriter{Out: stderr, NoColor: true}
	zcfg.PartsExclude = []string{
		zerolog.TimestampFieldName,
		zerolog.LevelFieldName,
	}
	zl := zerolog.New(zcfg)
	cliLogger = zerologr.New(&zl)
	runtimeLog.SetLogger(cliLogger)

	_, err = rootCmd.ExecuteC()
	return err
}

func resetCmdArgs() {
	applyArgs = applyFlags{}
	fmtArgs = fmtFlags{}
	buildArgs = buildFlags{output: "yaml"}
	deleteArgs = deleteFlags{}
	statusArgs = statusFlags{}
	inspectModuleArgs = inspectModuleFlags{}
	inspectResourcesArgs = inspectResourcesFlags{}
	inspectValuesArgs = inspectValuesFlags{}
	vetModArgs = vetModFlags{
		name: "default",
	}
	listArgs = listFlags{}
	listModArgs = listModFlags{withDigest: true, limit: 100}
	listArtifactArgs = listArtifactFlags{withDigest: true, limit: 100}
	pullModArgs = pullModFlags{}
	configShowModArgs = configModFlags{name: "module-name"}
	readmeShowModArgs = readmeModFlags{}
	pushModArgs = pushModFlags{}
	buildModArgs = buildModFlags{format: "oci-archive"}
	bundleArgs = bundleFlags{}
	bundleApplyArgs = bundleApplyFlags{}
	bundleVetArgs = bundleVetFlags{}
	bundleDelArgs = bundleDelFlags{}
	bundleBuildArgs = bundleBuildFlags{}
	vendorCrdArgs = vendorCrdFlags{}
	vendorK8sArgs = vendorK8sFlags{}
	pushArtifactArgs = pushArtifactFlags{
		path:        ".",
		contentType: "generic",
	}
	buildArtifactArgs = buildArtifactFlags{
		path:        ".",
		format:      "oci-archive",
		contentType: "generic",
	}
	pullArtifactArgs = pullArtifactFlags{}
	digestArtifactArgs = digestArtifactFlags{}
	runtimeBuildArgs = runtimeBuildFlags{}
	versionArgs = versionFlags{output: "yaml"}
}

func rnd(prefix string) string {
	runes := []rune("abcdefghijklmnopqrstuvwxyz1234567890")
	b := make([]rune, 5)
	for i := range b {
		b[i] = runes[rand.Intn(len(runes))]
	}
	return prefix + "-" + string(b)
}

func setupRegistryServer(ctx context.Context) error {
	// Registry config
	config := &configuration.Configuration{}
	config.Log.AccessLog.Disabled = true
	config.Log.Level = "error"
	port, err := freeport.GetFreePort()
	if err != nil {
		return fmt.Errorf("failed to get free port: %s", err)
	}
	logrus.SetOutput(io.Discard)
	dockerRegistry = fmt.Sprintf("localhost:%d", port)
	config.HTTP.Addr = fmt.Sprintf("127.0.0.1:%d", port)
	config.HTTP.DrainTimeout = time.Duration(10) * time.Second
	config.Storage = map[string]configuration.Parameters{"inmemory": map[string]any{}}
	dockerRegistry, err := registry.NewRegistry(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create docker registry: %w", err)
	}

	// Start Docker registry
	go func() {
		if err := dockerRegistry.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}()

	return nil
}
