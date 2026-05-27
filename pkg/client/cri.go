/**
# Copyright (c) Advanced Micro Devices, Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package k8sclient

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// Well-known CRI runtime socket paths (host-mounted under /host).
const (
	ContainerdRuntimeSocket    = "/host/run/containerd/containerd.sock"
	K3sContainerdRuntimeSocket = "/host/run/k3s/containerd/containerd.sock"
	CrioRuntimeSocket          = "/host/run/crio/crio.sock"
)

// CRIClient wraps a persistent gRPC connection to a CRI runtime socket.
// Use NewCRIClient to auto-detect the active runtime by probing sockets.
type CRIClient struct {
	conn   *grpc.ClientConn
	client pb.RuntimeServiceClient
	socket string
}

// NewCRIClient probes the given sockets to find the active CRI runtime.
// Detection uses ListPodSandbox — a successful response confirms the socket
// is a live CRI endpoint. When multiple sockets respond (e.g. k3s node with
// both stock containerd and k3s-containerd), the one with pods is preferred
// since that is kubelet's runtime.
// Returns an error if no active runtime is found.
func NewCRIClient(ctx context.Context) (*CRIClient, error) {
	sockets := []string{ContainerdRuntimeSocket, K3sContainerdRuntimeSocket, CrioRuntimeSocket}

	// Prefer the socket with pods (disambiguates k3s); fall back to any
	// socket that responded with 0 pods (freshly-started kubelet).
	// On k3s fresh start with both sockets at 0 pods, the first socket
	// wins — acceptable since pods appear within seconds of kubelet start.
	var fallback *CRIClient
	for _, socket := range sockets {
		if _, err := os.Stat(socket); err != nil {
			continue
		}
		conn, err := grpc.NewClient(
			"unix://"+socket,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			continue
		}

		client := pb.NewRuntimeServiceClient(conn)
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		resp, err := client.ListPodSandbox(probeCtx, &pb.ListPodSandboxRequest{})
		cancel()

		if err != nil || resp == nil {
			conn.Close()
			continue
		}

		if len(resp.Items) > 0 {
			logger.Log.Printf("CRI: detected runtime at %s (%d pods)", socket, len(resp.Items))
			if fallback != nil {
				fallback.Close()
			}
			return &CRIClient{conn: conn, client: client, socket: socket}, nil
		}

		// Socket responded but has 0 pods — keep as fallback
		if fallback == nil {
			fallback = &CRIClient{conn: conn, client: client, socket: socket}
		} else {
			conn.Close()
		}
	}

	if fallback != nil {
		logger.Log.Printf("CRI: no socket had pods; using %s (0 pods, kubelet may still be starting)", fallback.socket)
		return fallback, nil
	}
	return nil, fmt.Errorf("no active CRI runtime found at %v", sockets)
}

// Socket returns the path of the detected CRI runtime socket.
func (c *CRIClient) Socket() string {
	return c.socket
}

// LookupContainerID finds a pod's first running container ID via CRI
// ListPodSandbox and ListContainers. Kubelet sets io.kubernetes.pod.name and
// io.kubernetes.pod.namespace labels on every sandbox.
// Note: for multi-container pods, this returns the first running container.
func (c *CRIClient) LookupContainerID(ctx context.Context, podName, ns string) (string, error) {
	return CRILookupContainerID(ctx, c.client, podName, ns)
}

// ContainerStatus returns the verbose status of a container, including PID info.
func (c *CRIClient) ContainerStatus(ctx context.Context, containerID string) (*pb.ContainerStatusResponse, error) {
	return c.client.ContainerStatus(ctx, &pb.ContainerStatusRequest{
		ContainerId: containerID,
		Verbose:     true,
	})
}

// Close closes the underlying gRPC connection.
func (c *CRIClient) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// CRILookupContainerID finds a pod's first running container ID via CRI
// ListPodSandbox and ListContainers. This is the stateless version that takes
// a RuntimeServiceClient directly — used by both CRIClient and tests.
// Note: for multi-container pods, this returns the first running container
// (ordering is not guaranteed by the CRI spec). This matches the behavior of
// k8sApiClient.GetContainerIDforPod which also returns ContainerStatuses[0].
// For NIC PID resolution this is acceptable since we only need any container's
// PID to nsenter into the pod's network namespace (shared across containers).
func CRILookupContainerID(ctx context.Context, client pb.RuntimeServiceClient, podName, ns string) (string, error) {
	sandboxResp, err := client.ListPodSandbox(ctx, &pb.ListPodSandboxRequest{
		Filter: &pb.PodSandboxFilter{
			State: &pb.PodSandboxStateValue{
				State: pb.PodSandboxState_SANDBOX_READY,
			},
			LabelSelector: map[string]string{
				"io.kubernetes.pod.name":      podName,
				"io.kubernetes.pod.namespace": ns,
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to list pod sandboxes: %v", err)
	}
	if len(sandboxResp.Items) == 0 {
		return "", fmt.Errorf("no sandbox found for pod %s/%s", ns, podName)
	}

	sandboxID := sandboxResp.Items[0].Id
	containerResp, err := client.ListContainers(ctx, &pb.ListContainersRequest{
		Filter: &pb.ContainerFilter{
			PodSandboxId: sandboxID,
			State: &pb.ContainerStateValue{
				State: pb.ContainerState_CONTAINER_RUNNING,
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to list containers for sandbox %s: %v", sandboxID, err)
	}
	if len(containerResp.Containers) == 0 {
		return "", fmt.Errorf("no running containers in sandbox %s for pod %s/%s", sandboxID, ns, podName)
	}

	return containerResp.Containers[0].Id, nil
}
