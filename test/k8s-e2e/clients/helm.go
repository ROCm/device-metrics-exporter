/**
# Copyright (c) Advanced Micro Devices, Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the \"License\");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an \"AS IS\" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package clients

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	helm "github.com/mittwald/go-helm-client"
	helmValues "github.com/mittwald/go-helm-client/values"
	restclient "k8s.io/client-go/rest"
)

type HelmClientOpt func(client *HelmClient)

type HelmClient struct {
	client     helm.Client
	chart      string
	cache      string
	config     string
	ns         string
	restConfig *restclient.Config
	relName    string
}

func WithNameSpaceOption(namespace string) HelmClientOpt {
	return func(c *HelmClient) {
		c.ns = namespace
	}
}

func WithKubeConfigOption(kubeconf *restclient.Config) HelmClientOpt {
	return func(c *HelmClient) {
		c.restConfig = kubeconf
	}
}

func NewHelmClient(opts ...HelmClientOpt) (*HelmClient, error) {
	client := &HelmClient{}
	for _, opt := range opts {
		opt(client)
	}

	var err error
	client.cache, err = os.MkdirTemp("", ".hcache")
	if err != nil {
		return nil, err
	}

	client.config, err = os.MkdirTemp("", ".hconfig")
	if err != nil {
		return nil, err
	}
	restConfOptions := &helm.RestConfClientOptions{
		Options: &helm.Options{
			Namespace:        client.ns,
			RepositoryConfig: client.config,
			Debug:            true,
			RepositoryCache:  client.cache,
			DebugLog: func(format string, v ...interface{}) {
				log.Printf(format, v...)
			},
		},
		RestConfig: client.restConfig,
	}

	helmClient, err := helm.NewClientFromRestConf(restConfOptions)
	if err != nil {
		return nil, err
	}
	client.client = helmClient
	return client, nil
}

func (h *HelmClient) InstallChart(ctx context.Context, chart string, params []string) (string, error) {
	values := helmValues.Options{
		Values: params,
	}

	chartSpec := &helm.ChartSpec{
		ReleaseName:   "e2e-test-k8s",
		ChartName:     chart,
		Namespace:     h.ns,
		GenerateName:  false,
		Wait:          true,
		Timeout:       5 * time.Minute,
		CleanupOnFail: false,
		DryRun:        false,
		ValuesOptions: values,
	}

	resp, err := h.client.InstallChart(ctx, chartSpec, nil)
	if err != nil {
		return "", err
	}
	log.Printf("helm chart install resp: %+v", resp)
	h.relName = resp.Name
	return resp.Name, err
}

func (h *HelmClient) UninstallChart() error {
	if h.relName == "" {
		return fmt.Errorf("helm chart is not installed by client")
	}
	return h.client.UninstallReleaseByName(h.relName)
}

func (h *HelmClient) Cleanup() {
	err := os.RemoveAll(h.cache)
	if err != nil {
		log.Printf("failed to delete directory %s; err: %v", h.cache, err)
	}

	err = os.RemoveAll(h.config)
	if err != nil {
		log.Printf("failed to delete directory %s; err: %v", h.config, err)
	}
}
