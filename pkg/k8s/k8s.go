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

package k8s

import (
	"context"
	"fmt"
	"strings"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/gpumetrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	kube "k8s.io/kubelet/pkg/apis/podresources/v1alpha1"

	"os"
)

const PodResourceSocket = "/var/lib/kubelet/pod-resources/kubelet.sock"
const amdGpuResourceName = "amd.com/gpu"

var ExportLabels = map[string]bool{
	gpumetrics.GPUMetricLabel_POD.String():       true,
	gpumetrics.GPUMetricLabel_NAMESPACE.String(): true,
	gpumetrics.GPUMetricLabel_CONTAINER.String(): true,
}

type PodResourceInfo struct {
	Pod       string
	Namespace string
	Container string
}

type PodResourcesService interface {
	ListPods(ctx context.Context) (map[string]PodResourceInfo, error)
	CheckExportLabels(labels map[string]bool) bool
	Close() error
}

type podResourcesClient struct {
	clientConn *grpc.ClientConn
}

func IsKubernetes() bool {
	if s := os.Getenv("KUBERNETES_SERVICE_HOST"); s != "" {
		return true
	}
	if _, err := os.Stat(PodResourceSocket); err == nil {
		return true
	}
	return false
}

func NewClient() (PodResourcesService, error) {
	if _, err := os.Stat(PodResourceSocket); err != nil {
		return nil, fmt.Errorf("no kubelet, %v", err)
	}
	client, err := grpc.NewClient("unix://"+PodResourceSocket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("kubelet socket error, %v", err)
	}
	return &podResourcesClient{clientConn: client}, nil

}

func (pr *podResourcesClient) ListPods(ctx context.Context) (map[string]PodResourceInfo, error) {
	prCl := kube.NewPodResourcesListerClient(pr.clientConn)
	resp, err := prCl.List(ctx, &kube.ListPodResourcesRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pod resources, %v", err)
	}

	podInfo := make(map[string]PodResourceInfo)
	for _, pod := range resp.PodResources {
		for _, container := range pod.Containers {
			for _, devs := range container.GetDevices() {
				if devs.ResourceName == amdGpuResourceName {
					for _, devId := range devs.DeviceIds {
						podInfo[strings.ToLower(devId)] = PodResourceInfo{
							Pod:       pod.Name,
							Namespace: pod.Namespace,
							Container: container.Name,
						}
					}
				}
			}
		}
	}
	return podInfo, nil
}

func (cl *podResourcesClient) CheckExportLabels(labels map[string]bool) bool {
	for k := range ExportLabels {
		if ok := labels[k]; ok {
			return true
		}
	}
	return false
}

func (cl *podResourcesClient) Close() error {
	return cl.clientConn.Close()
}
