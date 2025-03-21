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

package k8e2e

import (
	"context"
	"flag"
	"log"
	"path/filepath"

	"testing"

	"github.com/ROCm/device-metrics-exporter/test/k8s-e2e/clients"
	"github.com/stretchr/testify/assert"
	. "gopkg.in/check.v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var kubeConfig = flag.String("kubeconfig", filepath.Join(homedir.HomeDir(), ".kube", "config"), "absolute path to the kubeconfig file")
var helmChart = flag.String("helmchart", "", "helmchart")
var exporterNS = flag.String("namespace", "kube-amd-gpu", "namespace")
var registry = flag.String("registry", "docker.io/rocm/device-metrics-exporter", "exporter container registry")
var imageTag = flag.String("imagetag", "latest", "exporter image version/tag")

// All the test config, state and any helper caches for running this test
// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) {
	TestingT(t)
}

var _ = Suite(&E2ESuite{})

func (s *E2ESuite) SetUpSuite(c *C) {
	log.Print("setupSuite:")
	s.helmChart = *helmChart
	s.kubeconfig = *kubeConfig
	s.ns = *exporterNS
	s.registry = *registry
	s.imageTag = *imageTag
	ctx := context.Background()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", s.kubeconfig)
	assert.NoError(c, err)
	s.restConfig = config

	// creates the clientset
	cs, err := clients.NewK8sClient(config)
	assert.NoError(c, err)

	s.k8sclient = cs
	// create namespace for test
	err = s.k8sclient.CreateNamespace(ctx, s.ns)
	assert.NoError(c, err)

	hClient, err := clients.NewHelmClient(
		clients.WithNameSpaceOption(s.ns),
		clients.WithKubeConfigOption(config),
	)
	assert.NoError(c, err)
	s.helmClient = hClient
}

func (s *E2ESuite) TearDownSuite(c *C) {
	log.Print("cleaning setup after test")
	err := s.k8sclient.DeleteNamespace(context.Background(), s.ns)
	assert.NoError(c, err)
	s.helmClient.Cleanup()
}
