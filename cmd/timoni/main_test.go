package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/distribution/distribution/v3/configuration"
	dcontext "github.com/distribution/distribution/v3/context"
	"github.com/distribution/distribution/v3/registry"
	_ "github.com/distribution/distribution/v3/registry/auth/htpasswd"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/mattn/go-shellwords"
	"github.com/phayes/freeport"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var (
	envTestClient  client.Client
	dockerRegistry string
)

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

	tmpFilename := filepath.Join(tmpDir, rnd("kubeconfig", 5))
	if err := os.WriteFile(tmpFilename, kubeConfig, 0644); err != nil {
		panic(err)
	}

	envTestClient, err = client.New(testEnv.Config, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		panic(fmt.Sprintf("failed to create k8s client: %v", err))
	}

	kubeconfigArgs.KubeConfig = &tmpFilename

	code := m.Run()
	testEnv.Stop()
	os.Exit(code)
}

func executeCommand(cmd string) (string, error) {
	defer resetCmdArgs()
	args, err := shellwords.Parse(cmd)
	if err != nil {
		return "", err
	}

	buf := new(bytes.Buffer)

	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)

	logger.stderr = rootCmd.ErrOrStderr()

	_, err = rootCmd.ExecuteC()
	result := buf.String()

	return result, err
}

func resetCmdArgs() {
	applyArgs = applyFlags{}
	buildArgs = buildFlags{}
	deleteArgs = deleteFlags{}
	inspectModuleArgs = inspectModuleFlags{}
	inspectResourcesArgs = inspectResourcesFlags{}
	inspectValuesArgs = inspectValuesFlags{}
	lintModArgs = lintModFlags{}
	listArgs = listFlags{}
	pullModArgs = pullModFlags{}
	pushModArgs = pushModFlags{}
}

func rnd(prefix string, n int) string {
	runes := []rune("abcdefghijklmnopqrstuvwxyz1234567890")
	b := make([]rune, n)
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
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	dcontext.SetDefaultLogger(logrus.NewEntry(logger))
	port, err := freeport.GetFreePort()
	if err != nil {
		return fmt.Errorf("failed to get free port: %s", err)
	}

	dockerRegistry = fmt.Sprintf("localhost:%d", port)
	config.HTTP.Addr = fmt.Sprintf("127.0.0.1:%d", port)
	config.HTTP.DrainTimeout = time.Duration(10) * time.Second
	config.Storage = map[string]configuration.Parameters{"inmemory": map[string]interface{}{}}
	dockerRegistry, err := registry.NewRegistry(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create docker registry: %w", err)
	}

	// Start Docker registry
	go dockerRegistry.ListenAndServe()

	return nil
}
