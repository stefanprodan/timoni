package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
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
	ctrl "sigs.k8s.io/controller-runtime"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var (
	dockerReg string
)

func TestMain(m *testing.M) {
	ctx := ctrl.SetupSignalHandler()
	err := setupRegistryServer(ctx)
	if err != nil {
		panic(fmt.Sprintf("failed to start docker registry: %s", err))
	}

	code := m.Run()
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
	lintArgs = lintFlags{}
	listArgs = listFlags{}
	pullArgs = pullFlags{}
	pushArgs = pushFlags{}
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

	dockerReg = fmt.Sprintf("localhost:%d", port)
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
