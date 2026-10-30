/*
Copyright 2024 Stefan Prodan

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

package testutils

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// credentialEnvVars are the environment variables that registry clients
// read credentials from in addition to DOCKER_CONFIG.
var credentialEnvVars = []string{"DOCKER_AUTH_CONFIG", "REGISTRY_AUTH_FILE"}

// NewTestRegistry starts an in-memory OCI registry on a random loopback port.
// Blobs are stored per repository, so a blob pushed to one repository is not
// visible from another. It returns the registry address as localhost:<port>,
// which registry clients reach over plain HTTP, and a function that shuts the
// registry down.
func NewTestRegistry() (string, func()) {
	srv := httptest.NewServer(registry.New(
		registry.Logger(log.New(io.Discard, "", 0)),
		registry.WithBlobHandler(&repoBlobHandler{blobs: map[string][]byte{}}),
	))
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		srv.Close()
		panic(err)
	}
	return net.JoinHostPort("localhost", port), srv.Close
}

// SetupTestRegistry starts an in-memory OCI registry and isolates the Docker
// config for the duration of the test. It returns the registry address.
func (t *WithT) SetupTestRegistry() string {
	t.Helper()

	dir := t.TempDir()
	if err := writeEmptyDockerConfig(dir); err != nil {
		t.Fatalf("failed to write docker config: %s", err)
	}
	t.Setenv("DOCKER_CONFIG", dir)
	for _, key := range credentialEnvVars {
		t.Setenv(key, "")
	}

	addr, stop := NewTestRegistry()
	t.Cleanup(stop)
	return addr
}

// IsolateDockerConfig points DOCKER_CONFIG at a new temporary directory that
// holds an empty config.json and unsets DOCKER_AUTH_CONFIG and
// REGISTRY_AUTH_FILE, so registry clients use anonymous auth instead of the
// user's credentials. It is meant for TestMain; tests should use
// SetupTestRegistry. The returned function restores the previous environment
// and removes the directory.
func IsolateDockerConfig() (func(), error) {
	dir, err := os.MkdirTemp("", "timoni-docker-config")
	if err != nil {
		return nil, err
	}

	restores := []func(){saveEnv("DOCKER_CONFIG")}
	for _, key := range credentialEnvVars {
		restores = append(restores, saveEnv(key))
	}
	cleanup := func() {
		for _, restore := range restores {
			restore()
		}
		_ = os.RemoveAll(dir)
	}

	if err := writeEmptyDockerConfig(dir); err != nil {
		cleanup()
		return nil, err
	}
	if err := os.Setenv("DOCKER_CONFIG", dir); err != nil {
		cleanup()
		return nil, err
	}
	for _, key := range credentialEnvVars {
		if err := os.Unsetenv(key); err != nil {
			cleanup()
			return nil, err
		}
	}

	return cleanup, nil
}

// writeEmptyDockerConfig writes a config.json with no credentials to dir.
func writeEmptyDockerConfig(dir string) error {
	return os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}"), 0o600)
}

// saveEnv records the current state of the variable and returns a function
// that restores it, unsetting the variable if it was not set.
func saveEnv(key string) func() {
	prev, had := os.LookupEnv(key)
	return func() {
		if had {
			_ = os.Setenv(key, prev)
		} else {
			_ = os.Unsetenv(key)
		}
	}
}

// repoBlobHandler is an in-memory registry blob store keyed by repository
// and digest.
type repoBlobHandler struct {
	mu    sync.Mutex
	blobs map[string][]byte
}

// blobKey returns the store key for a blob. The registry passes the
// repository as "<repo>/blobs" when it completes a chunked upload and as
// "<repo>" everywhere else, so the suffix is trimmed.
func blobKey(repo string, h v1.Hash) string {
	return strings.TrimSuffix(repo, "/blobs") + "@" + h.String()
}

// Stat returns the size of the blob in the repository.
func (b *repoBlobHandler) Stat(_ context.Context, repo string, h v1.Hash) (int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	data, ok := b.blobs[blobKey(repo, h)]
	if !ok {
		return 0, registry.ErrNotFound
	}
	return int64(len(data)), nil
}

// Get returns the content of the blob in the repository.
func (b *repoBlobHandler) Get(_ context.Context, repo string, h v1.Hash) (io.ReadCloser, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	data, ok := b.blobs[blobKey(repo, h)]
	if !ok {
		return nil, registry.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// Put stores the blob in the repository.
func (b *repoBlobHandler) Put(_ context.Context, repo string, h v1.Hash, rc io.ReadCloser) error {
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.blobs[blobKey(repo, h)] = data
	return nil
}

// Delete removes the blob from the repository.
func (b *repoBlobHandler) Delete(_ context.Context, repo string, h v1.Hash) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := blobKey(repo, h)
	if _, ok := b.blobs[key]; !ok {
		return registry.ErrNotFound
	}
	delete(b.blobs, key)
	return nil
}
