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
	"fmt"

	"github.com/fluxcd/pkg/ssa"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

var owner = ssa.Owner{
	Field: "timoni",
	Group: "modules.timoni.sh",
}

func newManager(owner ssa.Owner) (*ssa.ResourceManager, error) {

	kubeClient, err := newKubeClient(kubeconfigArgs)
	if err != nil {
		return nil, fmt.Errorf("client init failed: %w", err)
	}

	statusPoller, err := newKubeStatusPoller(kubeconfigArgs)
	if err != nil {
		return nil, fmt.Errorf("status poller init failed: %w", err)
	}

	return ssa.NewResourceManager(kubeClient, statusPoller, owner), nil
}

func newScheme() *apiruntime.Scheme {
	scheme := apiruntime.NewScheme()
	_ = apiextensionsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	return scheme
}

func newKubeClient(rcg genericclioptions.RESTClientGetter) (client.WithWatch, error) {
	cfg, err := newKubeConfig(rcg)
	if err != nil {
		return nil, fmt.Errorf("kubernetes client initialization failed: %w", err)
	}

	kubeClient, err := client.NewWithWatch(cfg, client.Options{
		Scheme: newScheme(),
	})
	if err != nil {
		return nil, fmt.Errorf("kubernetes client initialization failed: %w", err)
	}

	return kubeClient, nil
}

func newKubeStatusPoller(rcg genericclioptions.RESTClientGetter) (*polling.StatusPoller, error) {
	kubeConfig, err := newKubeConfig(rcg)
	if err != nil {
		return nil, err
	}

	restMapper, err := apiutil.NewDynamicRESTMapper(kubeConfig)
	if err != nil {
		return nil, err
	}
	c, err := client.New(kubeConfig, client.Options{Mapper: restMapper})
	if err != nil {
		return nil, err
	}

	return polling.NewStatusPoller(c, restMapper, polling.Options{}), nil
}

func newKubeConfig(rcg genericclioptions.RESTClientGetter) (*rest.Config, error) {
	cfg, err := rcg.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("kubeconfig load failed: %w", err)
	}

	cfg.QPS = 50
	cfg.Burst = 100

	return cfg, nil
}
