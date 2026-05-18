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
	"strings"
	"testing"

	"google.golang.org/grpc"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"
)

type mockRuntimeServiceClient struct {
	pb.RuntimeServiceClient
	listPodSandboxResp *pb.ListPodSandboxResponse
	listPodSandboxErr  error
	listContainersResp *pb.ListContainersResponse
	listContainersErr  error
}

func (m *mockRuntimeServiceClient) ListPodSandbox(_ context.Context, _ *pb.ListPodSandboxRequest, _ ...grpc.CallOption) (*pb.ListPodSandboxResponse, error) {
	return m.listPodSandboxResp, m.listPodSandboxErr
}

func (m *mockRuntimeServiceClient) ListContainers(_ context.Context, _ *pb.ListContainersRequest, _ ...grpc.CallOption) (*pb.ListContainersResponse, error) {
	return m.listContainersResp, m.listContainersErr
}

func TestCRIClientLookupContainerID(t *testing.T) {
	mock := &mockRuntimeServiceClient{
		listPodSandboxResp: &pb.ListPodSandboxResponse{
			Items: []*pb.PodSandbox{{Id: "sandbox-1"}},
		},
		listContainersResp: &pb.ListContainersResponse{
			Containers: []*pb.Container{{Id: "ctr-1"}},
		},
	}
	c := &CRIClient{client: mock, socket: "/test/socket"}

	ctx := context.Background()
	id, err := c.LookupContainerID(ctx, "pod-1", "ns-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "ctr-1" {
		t.Fatalf("expected ctr-1, got %s", id)
	}
	if c.Socket() != "/test/socket" {
		t.Fatalf("expected /test/socket, got %s", c.Socket())
	}
}

func TestCRILookupContainerID(t *testing.T) {
	tests := []struct {
		name        string
		mock        *mockRuntimeServiceClient
		wantID      string
		wantErr     bool
		errContains string
	}{
		{
			name: "pod found with single running container",
			mock: &mockRuntimeServiceClient{
				listPodSandboxResp: &pb.ListPodSandboxResponse{
					Items: []*pb.PodSandbox{
						{Id: "sandbox-123"},
					},
				},
				listContainersResp: &pb.ListContainersResponse{
					Containers: []*pb.Container{
						{Id: "container-abc"},
					},
				},
			},
			wantID:  "container-abc",
			wantErr: false,
		},
		{
			name: "no sandbox found",
			mock: &mockRuntimeServiceClient{
				listPodSandboxResp: &pb.ListPodSandboxResponse{
					Items: []*pb.PodSandbox{},
				},
			},
			wantErr:     true,
			errContains: "no sandbox found",
		},
		{
			name: "sandbox found but no running containers",
			mock: &mockRuntimeServiceClient{
				listPodSandboxResp: &pb.ListPodSandboxResponse{
					Items: []*pb.PodSandbox{
						{Id: "sandbox-456"},
					},
				},
				listContainersResp: &pb.ListContainersResponse{
					Containers: []*pb.Container{},
				},
			},
			wantErr:     true,
			errContains: "no running containers",
		},
		{
			name: "ListPodSandbox returns error",
			mock: &mockRuntimeServiceClient{
				listPodSandboxErr: fmt.Errorf("connection refused"),
			},
			wantErr:     true,
			errContains: "failed to list pod sandboxes",
		},
		{
			name: "ListContainers returns error",
			mock: &mockRuntimeServiceClient{
				listPodSandboxResp: &pb.ListPodSandboxResponse{
					Items: []*pb.PodSandbox{
						{Id: "sandbox-789"},
					},
				},
				listContainersErr: fmt.Errorf("internal error"),
			},
			wantErr:     true,
			errContains: "failed to list containers",
		},
		{
			name: "multi-container pod returns first container",
			mock: &mockRuntimeServiceClient{
				listPodSandboxResp: &pb.ListPodSandboxResponse{
					Items: []*pb.PodSandbox{
						{Id: "sandbox-multi"},
					},
				},
				listContainersResp: &pb.ListContainersResponse{
					Containers: []*pb.Container{
						{Id: "container-first"},
						{Id: "container-second"},
					},
				},
			},
			wantID:  "container-first",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			gotID, err := CRILookupContainerID(ctx, tt.mock, "test-pod", "default")

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotID != tt.wantID {
				t.Fatalf("expected container ID %q, got %q", tt.wantID, gotID)
			}
		})
	}
}
