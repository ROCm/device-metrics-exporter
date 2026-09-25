/*
Copyright (c) Advanced Micro Devices, Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the \"License\");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an \"AS IS\" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package scheduler

import (
	"testing"

	kube "k8s.io/kubelet/pkg/apis/podresources/v1"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
)

// draClaimPod returns a pod that references one DRA claim holding the given
// devices. Several pods can reference the same claim.
func draClaimPod(pod, namespace, container string, devices ...string) *kube.PodResources {
	claimResources := make([]*kube.ClaimResource, 0, len(devices))
	for _, device := range devices {
		claimResources = append(claimResources, &kube.ClaimResource{
			DriverName: globals.AMDGPUDriverName,
			PoolName:   "node-1",
			DeviceName: device,
		})
	}
	return &kube.PodResources{
		Name:      pod,
		Namespace: namespace,
		Containers: []*kube.ContainerResources{{
			Name: container,
			DynamicResources: []*kube.DynamicResource{{
				ClaimName:      "shared-text-image-pair-amd",
				ClaimNamespace: namespace,
				ClaimResources: claimResources,
			}},
		}},
	}
}

// pluginPod returns a pod allocated the given devices through the device plugin.
func pluginPod(pod, namespace, container string, devices ...string) *kube.PodResources {
	return &kube.PodResources{
		Name:      pod,
		Namespace: namespace,
		Containers: []*kube.ContainerResources{{
			Name: container,
			Devices: []*kube.ContainerDevices{{
				ResourceName: globals.AMDGPUResourcePrefix + "gpu",
				DeviceIds:    devices,
			}},
		}},
	}
}

// podNames returns the pods consuming one device.
func podNames(consumers Workloads) []string {
	names := []string{}
	for _, wl := range consumers {
		info, ok := wl.Info.(PodResourceInfo)
		if !ok {
			continue
		}
		names = append(names, info.Pod)
	}
	return names
}

// A DRA claim can be referenced by more than one pod and each referencing pod
// consumes every device the claim holds. Every one of them must be listed: an
// entry naming only one consumer leaves the other with no series at all, and
// which one it names follows the order the kubelet happens to list the pods in.
func TestEveryConsumerOfADraClaimIsListed(t *testing.T) {
	got := workloadsFromPodResources([]*kube.PodResources{
		draClaimPod("llm-a", "ai", "server", "gpu-0-0", "gpu-1-0"),
		draClaimPod("llm-b", "ai", "server", "gpu-0-0", "gpu-1-0"),
	})

	if len(got) != 2 {
		t.Fatalf("the claim's devices mapped to %d entries, want 2", len(got))
	}
	for _, device := range []string{"gpu-0-0", "gpu-1-0"} {
		consumers, found := got[device]
		if !found {
			t.Fatalf("device %s has no entry", device)
		}
		if names := podNames(consumers); len(names) != 2 {
			t.Errorf("device %s is consumed by %v, want both pods", device, names)
		}
	}
}

// A pod allocated a device by the device plugin names that one consumer.
func TestADevicePluginAllocationNamesItsConsumer(t *testing.T) {
	got := workloadsFromPodResources([]*kube.PodResources{
		pluginPod("llm", "ai", "server", "gpu-0"),
	})

	if names := podNames(got["gpu-0"]); len(names) != 1 || names[0] != "llm" {
		t.Errorf("device gpu-0 is consumed by %v, want llm", names)
	}
}

// Two containers of one pod are two consumers of the device they share.
func TestEachContainerIsAConsumerOfItsOwn(t *testing.T) {
	pod := pluginPod("llm", "ai", "server", "gpu-0")
	pod.Containers = append(pod.Containers, &kube.ContainerResources{
		Name: "sidecar",
		Devices: []*kube.ContainerDevices{{
			ResourceName: globals.AMDGPUResourcePrefix + "gpu",
			DeviceIds:    []string{"gpu-0"},
		}},
	})

	got := workloadsFromPodResources([]*kube.PodResources{pod})

	if consumers := got["gpu-0"]; len(consumers) != 2 {
		t.Errorf("device gpu-0 is consumed by %v, want both containers", podNames(consumers))
	}
}

// The same consumer reported twice is one consumer, so a device does not collect
// duplicate series.
func TestARepeatedConsumerIsListedOnce(t *testing.T) {
	var wls Workloads
	wl := Workload{Type: Kubernetes, Info: PodResourceInfo{Pod: "llm", Namespace: "ai", Container: "server"}}
	wls.Append(wl)
	wls.Append(wl)
	wls.Append(Workload{Type: Kubernetes, Info: PodResourceInfo{Pod: "llm", Namespace: "ai", Container: "sidecar"}})

	if len(wls) != 2 {
		t.Errorf("the list holds %d consumers, want 2", len(wls))
	}
}
