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
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	kube "k8s.io/kubelet/pkg/apis/podresources/v1"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
)

// fakePodResources is a PodResourcesLister server with a call counter and a
// settable response and error.
type fakePodResources struct {
	kube.UnimplementedPodResourcesListerServer
	calls atomic.Int64
	mu    sync.Mutex
	resp  *kube.ListPodResourcesResponse
	err   error
}

func (f *fakePodResources) List(context.Context, *kube.ListPodResourcesRequest) (*kube.ListPodResourcesResponse, error) {
	f.calls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.resp, f.err
}

func (f *fakePodResources) set(resp *kube.ListPodResourcesResponse, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resp, f.err = resp, err
}

// fakeKubelet serves a fakePodResources on a unix socket in a temp dir and can
// be stopped and restarted on the same path.
type fakeKubelet struct {
	t    *testing.T
	sock string
	fake *fakePodResources
	srv  *grpc.Server
}

func newFakeKubelet(t *testing.T, resp *kube.ListPodResourcesResponse) *fakeKubelet {
	t.Helper()
	logger.Init(true)
	k := &fakeKubelet{t: t, sock: filepath.Join(t.TempDir(), "kubelet.sock"), fake: &fakePodResources{resp: resp}}
	k.start()
	t.Cleanup(k.stop)
	return k
}

func (k *fakeKubelet) start() {
	k.t.Helper()
	lis, err := net.Listen("unix", k.sock)
	if err != nil {
		k.t.Fatal(err)
	}
	k.srv = grpc.NewServer()
	kube.RegisterPodResourcesListerServer(k.srv, k.fake)
	go func() { _ = k.srv.Serve(lis) }()
}

func (k *fakeKubelet) stop() {
	if k.srv != nil {
		k.srv.Stop()
		k.srv = nil
	}
}

// countDials wraps dialKubelet with a counter for the duration of the test.
// It swaps a package variable, so tests using it must not call t.Parallel().
func countDials(t *testing.T) *atomic.Int64 {
	t.Helper()
	var n atomic.Int64
	orig := dialKubelet
	dialKubelet = func(socket string) (*grpc.ClientConn, error) {
		n.Add(1)
		return orig(socket)
	}
	t.Cleanup(func() { dialKubelet = orig })
	return &n
}

func pluginPod(name, ns, ctr, resource string, ids ...string) *kube.PodResources {
	return &kube.PodResources{Name: name, Namespace: ns, Containers: []*kube.ContainerResources{{
		Name: ctr, Devices: []*kube.ContainerDevices{{ResourceName: resource, DeviceIds: ids}}}}}
}

func draPod(name, ns, ctr, driver string, devs ...string) *kube.PodResources {
	var claims []*kube.ClaimResource
	for _, d := range devs {
		claims = append(claims, &kube.ClaimResource{DriverName: driver, DeviceName: d})
	}
	return &kube.PodResources{Name: name, Namespace: ns, Containers: []*kube.ContainerResources{{
		Name: ctr, DynamicResources: []*kube.DynamicResource{{ClaimResources: claims}}}}}
}

func podWl(pod, ns, ctr string) Workload {
	return Workload{Type: Kubernetes, Info: PodResourceInfo{Pod: pod, Namespace: ns, Container: ctr}}
}

var onePodResp = &kube.ListPodResourcesResponse{PodResources: []*kube.PodResources{
	pluginPod("pod0", "ns0", "ctr0", "amd.com/gpu", "0000:01:00.0")}}

var onePodWant = map[string]Workload{"0000:01:00.0": podWl("pod0", "ns0", "ctr0")}

// TestK8sListWorkloadsFakeServer (case 6): the map built from plugin and DRA
// responses is unchanged: device ids lower-cased, non-AMD resources ignored,
// and the first matching mode (plugin or DRA) locks out the other.
func TestK8sListWorkloadsFakeServer(t *testing.T) {
	for _, tc := range []struct {
		name string
		resp []*kube.PodResources
		want map[string]Workload
	}{
		{
			name: "plugin",
			resp: []*kube.PodResources{
				pluginPod("pod0", "ns0", "ctr0", "amd.com/gpu", "0000:0A:00.0", "0000:0b:00.0"),
				pluginPod("pod1", "ns1", "ctr1", "nvidia.com/gpu", "0000:0c:00.0"),
			},
			want: map[string]Workload{
				"0000:0a:00.0": podWl("pod0", "ns0", "ctr0"),
				"0000:0b:00.0": podWl("pod0", "ns0", "ctr0"),
			},
		},
		{
			name: "dra",
			resp: []*kube.PodResources{
				draPod("pod0", "ns0", "ctr0", "gpu.amd.com", "GPU-Card1-Render128"),
				draPod("pod1", "ns1", "ctr1", "other.com", "dev9"),
			},
			want: map[string]Workload{"gpu-card1-render128": podWl("pod0", "ns0", "ctr0")},
		},
		{
			name: "plugin locks out dra",
			resp: []*kube.PodResources{
				pluginPod("pod0", "ns0", "ctr0", "amd.com/gpu", "0000:01:00.0"),
				draPod("pod1", "ns1", "ctr1", "gpu.amd.com", "gpu-x"),
			},
			want: map[string]Workload{"0000:01:00.0": podWl("pod0", "ns0", "ctr0")},
		},
		{
			name: "dra locks out plugin",
			resp: []*kube.PodResources{
				draPod("pod1", "ns1", "ctr1", "gpu.amd.com", "gpu-x"),
				pluginPod("pod0", "ns0", "ctr0", "amd.com/gpu", "0000:01:00.0"),
			},
			want: map[string]Workload{"gpu-x": podWl("pod1", "ns1", "ctr1")},
		},
		{name: "empty", want: map[string]Workload{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			k := newFakeKubelet(t, &kube.ListPodResourcesResponse{PodResources: tc.resp})
			cl, err := newKubernetesClient(context.Background(), k.sock)
			if err != nil {
				t.Fatal(err)
			}
			defer cl.Close()
			got, err := cl.ListWorkloads()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ListWorkloads = %v; want %v", got, tc.want)
			}
			if n := k.fake.calls.Load(); n != 1 {
				t.Errorf("server List calls = %d; want 1", n)
			}
		})
	}
}

// TestK8sListWorkloadsServerError: a non-connection error is returned as is,
// with no re-dial and no retry.
func TestK8sListWorkloadsServerError(t *testing.T) {
	dials := countDials(t)
	k := newFakeKubelet(t, nil)
	k.fake.set(nil, status.Error(codes.Internal, "boom"))
	cl, err := newKubernetesClient(context.Background(), k.sock)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	got, err := cl.ListWorkloads()
	if err == nil || got != nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("ListWorkloads = %v, %v; want nil, boom error", got, err)
	}
	if d, c := dials.Load(), k.fake.calls.Load(); d != 1 || c != 1 {
		t.Errorf("dials = %d, server calls = %d; want 1, 1", d, c)
	}
}

func TestK8sNewClientNoSocket(t *testing.T) {
	if _, err := newKubernetesClient(context.Background(), filepath.Join(t.TempDir(), "none.sock")); err == nil {
		t.Fatal("newKubernetesClient on a missing socket succeeded")
	}
}

// TestK8sClientRecreatesClosedConn (case 85): a client whose connection was
// closed (what gpuagent's Close() did on every reconnect) re-dials its socket
// on the next List and returns the right map; a kubelet restart also recovers.
func TestK8sClientRecreatesClosedConn(t *testing.T) {
	dials := countDials(t)
	k := newFakeKubelet(t, onePodResp)
	cl, err := newKubernetesClient(context.Background(), k.sock)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	if got, err := cl.ListWorkloads(); err != nil || !reflect.DeepEqual(got, onePodWant) {
		t.Fatalf("ListWorkloads = %v, %v", got, err)
	}

	old := cl.conn()
	if err := cl.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := cl.ListWorkloads()
	if err != nil || !reflect.DeepEqual(got, onePodWant) {
		t.Fatalf("ListWorkloads after Close = %v, %v; want %v", got, err, onePodWant)
	}
	if cl.conn() == old {
		t.Error("connection was not re-created")
	}
	if d := dials.Load(); d != 2 {
		t.Errorf("dials = %d; want 2 (create + one re-dial)", d)
	}
	if got, err := cl.ListWorkloads(); err != nil || !reflect.DeepEqual(got, onePodWant) {
		t.Fatalf("ListWorkloads on re-created conn = %v, %v", got, err)
	}

	// kubelet restart: fails while down, recovers once it is back.
	k.stop()
	if _, err := cl.ListWorkloads(); err == nil {
		t.Error("ListWorkloads succeeded with the kubelet down")
	}
	k.start()
	deadline := time.Now().Add(10 * time.Second)
	for {
		got, err := cl.ListWorkloads()
		if err == nil && reflect.DeepEqual(got, onePodWant) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no recovery after kubelet restart: %v, %v", got, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestK8sClientConcurrentReconnect (case 86): two borrowers (gpuagent workload
// source and nicagent) share one client. Each time the connection is closed
// under them, the concurrent failures re-dial exactly once and both get the
// right map. Run with -race: clientConn is never written unsynchronized.
func TestK8sClientConcurrentReconnect(t *testing.T) {
	dials := countDials(t)
	k := newFakeKubelet(t, onePodResp)
	cl, err := newKubernetesClient(context.Background(), k.sock)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()

	const episodes, borrowers, perEpisode = 50, 2, 10
	for ep := 0; ep < episodes; ep++ {
		before := dials.Load()
		if err := cl.Close(); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make(chan error, borrowers*perEpisode)
		for b := 0; b < borrowers; b++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < perEpisode; i++ {
					got, err := cl.ListWorkloads()
					if err == nil && !reflect.DeepEqual(got, onePodWant) {
						err = status.Errorf(codes.DataLoss, "wrong map %v", got)
					}
					if err != nil {
						errs <- err
					}
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Errorf("episode %d: %v", ep, err)
		}
		if d := dials.Load() - before; d != 1 {
			t.Fatalf("episode %d: %d dials; want exactly 1", ep, d)
		}
	}
	if n := k.fake.calls.Load(); n < episodes*borrowers*perEpisode {
		t.Errorf("server calls = %d; want >= %d", n, episodes*borrowers*perEpisode)
	}
}

// TestK8sIsConnectionError: the closed-connection error is now a connection
// error (re-created), others are unchanged.
func TestK8sIsConnectionError(t *testing.T) {
	cl := &podResourcesClient{}
	for msg, want := range map[string]bool{
		"rpc error: code = Canceled desc = grpc: the client connection is closing": true,
		"connection refused":                        true,
		"read: connection reset by peer":            true,
		"transport is closing":                      true,
		"rpc error: code = Internal desc = boom":    false,
		"rpc error: code = DeadlineExceeded desc =": false,
	} {
		if got := cl.isConnectionError(status.Error(codes.Unknown, msg)); got != want {
			t.Errorf("isConnectionError(%q) = %v; want %v", msg, got, want)
		}
	}
}
