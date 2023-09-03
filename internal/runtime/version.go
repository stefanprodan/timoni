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

package runtime

import (
	"fmt"
	"time"

	"github.com/Masterminds/semver/v3"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
)

// ServerVersion retrieves and parses the Kubernetes server's version.
func ServerVersion(rcg genericclioptions.RESTClientGetter) (string, error) {
	cfg, err := rcg.ToRESTConfig()
	if err != nil {
		return "", fmt.Errorf("loading kubeconfig failed: %w", err)
	}

	cfg.Timeout = 5 * time.Second

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "", fmt.Errorf("initialising client failed: %w", err)
	}

	serverVer, err := kubeClient.Discovery().ServerVersion()
	if err != nil {
		return "", fmt.Errorf("reading server version failed: %w", err)
	}

	ver, err := semver.NewVersion(serverVer.GitVersion)
	if err != nil {
		return "", fmt.Errorf("parsing server version failed: %w", err)
	}

	return ver.String(), nil
}
