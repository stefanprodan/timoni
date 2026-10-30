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

package oci

import (
	"fmt"
	"math/rand"
	"os"
	"testing"

	"github.com/stefanprodan/timoni/internal/testutils"
)

var dockerRegistry string

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests sets up the OCI registry, runs the tests and tears the registry
// down. It returns the exit code of the test run.
func runTests(m *testing.M) int {
	restoreDockerConfig, err := testutils.IsolateDockerConfig()
	if err != nil {
		panic(fmt.Sprintf("failed to isolate docker config: %s", err))
	}
	defer restoreDockerConfig()

	var stopRegistry func()
	dockerRegistry, stopRegistry = testutils.NewTestRegistry()
	defer stopRegistry()

	return m.Run()
}

func rnd(prefix string) string {
	runes := []rune("abcdefghijklmnopqrstuvwxyz1234567890")
	b := make([]rune, 5)
	for i := range b {
		b[i] = runes[rand.Intn(len(runes))]
	}
	return prefix + "-" + string(b)
}
