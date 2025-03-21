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

package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/gpumetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	kube "k8s.io/kubelet/pkg/apis/podresources/v1alpha1"

	"os"
)

var KubernetesLabels = map[string]bool{
	gpumetrics.GPUMetricLabel_POD.String():       true,
	gpumetrics.GPUMetricLabel_NAMESPACE.String(): true,
	gpumetrics.GPUMetricLabel_CONTAINER.String(): true,
}

type podResourcesClient struct {
	clientConn *grpc.ClientConn
	ctx        context.Context // parent context
}

// NewKubernetesClient - creates a kubernetes schedler client
func NewKubernetesClient(ctx context.Context) (SchedulerClient, error) {
	if _, err := os.Stat(globals.PodResourceSocket); err != nil {
		logger.Log.Printf("no kubelet found")
		return nil, fmt.Errorf("no kubelet, %v", err)
	}
	client, err := grpc.NewClient("unix://"+globals.PodResourceSocket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Log.Printf("kubelet socket err: %v", err)
		return nil, fmt.Errorf("kubelet socket error, %v", err)
	}
	logger.Log.Printf("created k8s scheduler client")
	return &podResourcesClient{clientConn: client, ctx: ctx}, nil

}

func (pr *podResourcesClient) ListWorkloads() (map[string]interface{}, error) {
	prCl := kube.NewPodResourcesListerClient(pr.clientConn)
	ctx, cancel := context.WithTimeout(pr.ctx, time.Second*10)
	defer cancel()
	resp, err := prCl.List(ctx, &kube.ListPodResourcesRequest{})
	if err != nil {
		logger.Log.Printf("failed to list pod resources, %v", err)
		return nil, fmt.Errorf("failed to list pod resources, %v", err)
	}

	podInfo := make(map[string]interface{})
	for _, pod := range resp.PodResources {
		for _, container := range pod.Containers {
			for _, devs := range container.GetDevices() {
				if devs.ResourceName == globals.AMDGPUResourceLabel {
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
	for k := range KubernetesLabels {
		if ok := labels[k]; ok {
			return true
		}
	}
	return false
}

func (cl *podResourcesClient) Close() error {
	return cl.clientConn.Close()
}

func (cl *podResourcesClient) Type() SchedulerType {
	return Kubernetes
}
