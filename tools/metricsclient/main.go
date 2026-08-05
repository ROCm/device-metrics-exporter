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

// client/client.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/fsysdevice"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	k8sclient "github.com/ROCm/device-metrics-exporter/pkg/client"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/metricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
	"github.com/gofrs/uuid"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kube "k8s.io/kubelet/pkg/apis/podresources/v1"
)

// Global flags
var (
	jsonOutput  bool
	socketPath  string
	kubeConfig  string
	eccFilePath string
)

// printLabels prints labels in key=value format, sorted by key
func printLabels(labels map[string]string) {
	if len(labels) == 0 {
		fmt.Println("  (none)")
		return
	}
	var keys []string
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %s=%s\n", k, labels[k])
	}
}

func prettyPrintGPUState(resp *metricssvc.GPUStateResponse) {
	if jsonOutput {
		jsonData, err := json.Marshal(resp)
		if err != nil {
			fmt.Println("Error:", err)
			return
		}
		fmt.Println(string(jsonData))
		return
	}
	sortOp := make(map[string]*metricssvc.GPUState)
	for _, gs := range resp.GPUState {
		sortOp[gs.ID] = gs
	}
	fmt.Printf("%-10s %-40s %-10s %-30s\n",
		"ID", "UUID", "Health", "Associated Workload")
	fmt.Println("------------------------------------------------")
	for i := 0; i < len(sortOp); i++ {
		gs := sortOp[fmt.Sprintf("%d", i)]
		fmt.Printf("%-10v %-40s %-10v %+v\n", gs.ID, gs.UUID,
			gs.Health, gs.AssociatedWorkload)
	}
	fmt.Println("------------------------------------------------")
}

func prettyPrintErrResponse(resp *metricssvc.GPUErrorResponse) {
	jsonData, err := json.Marshal(resp)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Println(string(jsonData))
}

func send(socketPath string) error {
	conn, err := grpc.NewClient(
		socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()), // Use insecure credentials for simplicity
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	// create a new gRPC echo client through the compiled stub
	client := metricssvc.NewMetricsServiceClient(conn)

	resp, err := client.List(context.Background(), &emptypb.Empty{})
	if err != nil {
		return err
	}

	prettyPrintGPUState(resp)
	return nil
}

func get(socketPath, id string) error {
	conn, err := grpc.NewClient(
		socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()), // Use insecure credentials for simplicity
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	// create a new gRPC echo client through the compiled stub
	client := metricssvc.NewMetricsServiceClient(conn)

	// send an metricssvcrequest
	gpuReq := &metricssvc.GPUGetRequest{
		ID: []string{id},
	}
	_, err = client.GetGPUState(context.Background(), gpuReq)
	if err != nil {
		return err
	}

	// send an metricssvcrequest
	resp, err := client.GetGPUState(context.Background(),
		&metricssvc.GPUGetRequest{ID: gpuReq.ID})
	if err != nil {
		return err
	}
	prettyPrintGPUState(resp)

	return nil
}

func setError(socketPath, filepath string) error {

	// send an metricssvcrequest
	gpuUpdate := &metricssvc.GPUErrorRequest{}
	eccConfigs, err := os.ReadFile(filepath)
	if err != nil {
		fmt.Printf("err: %+v", err)
		return err
	} else {
		err = json.Unmarshal(eccConfigs, gpuUpdate)
		if err != nil {
			fmt.Printf("err: %+v", err)
			return err
		}
	}

	conn, err := grpc.NewClient(
		socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()), // Use insecure credentials for simplicity
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	// create a new gRPC echo client through the compiled stub
	client := metricssvc.NewMetricsServiceClient(conn)

	resp, err := client.SetError(context.Background(), gpuUpdate)
	if err != nil {
		return err
	}

	prettyPrintErrResponse(resp)

	return nil
}

func getGpuAgent(port, socketPath string, isJson bool, filter *amdgpu.GPUGetFilter) {
	addrString := ""
	if socketPath != "" {
		addrString = fmt.Sprintf("unix://%s", socketPath)
	} else {
		addrString = fmt.Sprintf("localhost:%s", port)
	}
	conn, err := grpc.NewClient(
		addrString,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		fmt.Printf("failed to connect to GPU agent: %v\n", err)
		return
	}
	defer conn.Close()

	client := amdgpu.NewGPUSvcClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.GPUGet(ctx, &amdgpu.GPUGetRequest{Filter: filter})
	if err != nil {
		fmt.Printf("GPUGet call failed: %v\n", err)
		return
	}
	// Sort the GPUs by Status.Index in ascending order
	gpus := resp.Response
	sort.Slice(gpus, func(i, j int) bool {
		return gpus[i].Status.Index < gpus[j].Status.Index
	})
	resp.Response = gpus

	toJsonString := func(v interface{}) string {
		data, err := json.Marshal(v)
		if err != nil {
			return ""
		}
		return string(data)
	}

	if isJson {
		jsonData, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			fmt.Printf("failed to marshal response: %v\n", err)
			return
		}
		fmt.Println(string(jsonData))
	} else {
		// Print each GPU as a row with fields
		for _, gpu := range resp.Response {
			fmt.Println(strings.Repeat("-", 40))
			fmt.Printf("Index : %v\n", gpu.Status.Index)
			fmt.Printf("Spec  : %s\n", toJsonString(gpu.Spec))
			fmt.Printf("Status: %s\n", toJsonString(gpu.Status))
			fmt.Printf("Stats : %s\n", toJsonString(gpu.Stats))
		}
		fmt.Println(strings.Repeat("-", 40))
	}
}

// determineGPUAgentConnection determines whether to connect to gpuagent via port or socket
// based on provided flags and socket file existence. Returns portStr, socketStr.
func determineGPUAgentConnection(port, socket string) (portStr, socketStr string) {
	if port != "" {
		portStr = port
		socketStr = ""
	} else {
		// Check if default socket file exists when no explicit option provided
		if socket == globals.GPUAgentDefaultSocketPath {
			if _, err := os.Stat(globals.GPUAgentDefaultSocketPath); err == nil {
				// Socket file exists, use socket-based connection
				portStr = ""
				socketStr = socket
			} else {
				// Socket file doesn't exist, default to IP:port
				portStr = fmt.Sprintf("%d", globals.GPUAgentPort)
				socketStr = ""
			}
		} else {
			// Explicit socket path provided, use it
			portStr = ""
			socketStr = socket
		}
	}
	return portStr, socketStr
}

func getDeviceMap() {
	devices, err := fsysdevice.FindAMDGPUDevices()
	if err != nil {
		fmt.Printf("device get error : %+v", err)
		return
	}
	fmt.Printf("Logical Device Map \n")
	for k, v := range devices {
		fmt.Printf("Render ID [%v] -> Device Name [%v]\n", k, v)
	}
}

func getPodResources() {
	if _, err := os.Stat(globals.PodResourceSocket); err != nil {
		fmt.Printf("no kubelet, %v", err)
		return
	}
	client, err := grpc.NewClient(
		"unix://"+globals.PodResourceSocket,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("kubelet socket error, %v", err)
		return
	}

	prCl := kube.NewPodResourcesListerClient(client)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	resp, err := prCl.List(ctx, &kube.ListPodResourcesRequest{})
	if err != nil {
		fmt.Printf("failed to list pod resources, %v", err)
		return
	}

	mode := "" // "plugin" or "dra"
	for _, pod := range resp.PodResources {
		for _, container := range pod.Containers {
			if mode != "dra" {
				for _, devs := range container.GetDevices() {
					if strings.HasPrefix(devs.ResourceName, globals.AMDGPUResourcePrefix) {
						for _, devId := range devs.DeviceIds {
							fmt.Printf("dev:ns/pod/container [{%v}%v/%v/%v]\n",
								devId, pod.Name, pod.Namespace, container.Name)
							mode = "plugin"
						}
					}
				}
			}
			if mode != "plugin" {
				for _, devs := range container.GetDynamicResources() {
					for _, claim := range devs.ClaimResources {
						if strings.HasPrefix(claim.DriverName, globals.AMDGPUDriverName) {
							fmt.Printf("dev:ns/pod/container [{%v}%v/%v/%v]\n",
								claim.DeviceName, pod.Name, pod.Namespace, container.Name)
							mode = "dra"
						}
					}
				}
			}
		}
	}

	if mode == "" {
		fmt.Printf("no associations found\n")
	}
	fmt.Printf("\npod resp:\n %+v\n", resp)
}

func getNodePods(kubeconfig string) {
	nodeName := utils.GetNodeName()
	if nodeName == "" {
		fmt.Println("not a k8s deployment")
		return
	}
	kc, err := k8sclient.NewClient(context.Background(), kubeconfig, nodeName)
	if err != nil {
		fmt.Printf("err: %+v", err)
		return
	}
	clientset := kc.GetClientSet()
	if clientset == nil {
		fmt.Printf("Invalid clientset")
		return
	}
	// List pods scheduled on the node
	podList, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
	})
	if err != nil {
		log.Fatalf("Failed to list pods on node: %v", err)
	}

	fmt.Printf("\nPods scheduled on node %s:\n", nodeName)
	for _, pod := range podList.Items {
		fmt.Printf("- %s/%s (Phase: %s)\n", pod.Namespace, pod.Name, pod.Status.Phase)
		fmt.Println("  Labels:")
		printLabels(pod.Labels)
		fmt.Println("  UID:", pod.ObjectMeta.UID)
		fmt.Println()
	}
}

func getNodeLabel(kubeconfig string) {
	nodeName := utils.GetNodeName()
	if nodeName == "" {
		fmt.Println("not a k8s deployment")
		return
	}
	kc, err := k8sclient.NewClient(context.Background(), kubeconfig, nodeName)
	if err != nil {
		fmt.Printf("err: %+v", err)
		return
	}
	clientset := kc.GetClientSet()
	if clientset == nil {
		fmt.Printf("Invalid clientset")
		return
	}
	node, err := clientset.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("err: %+v", err)
		return
	}
	// Extract and print the labels
	for key, value := range node.Labels {
		fmt.Printf("Label %s = %s\n", key, value)
	}
}

// MockCPEREntry represents a CPER entry for mock inband RAS
type MockCPEREntry struct {
	GPU  string   `json:"gpu"`
	AFId []uint64 `json:"afid"`
}

// MockInbandError represents the mock inband error structure
type MockInbandError struct {
	CPER []MockCPEREntry `json:"cper"`
}

// setupMockInbandRAS creates the mock inband RAS error_list file
func setupMockInbandRAS(port, socketPath string) error {
	// Determine connection method: socket is default unless port is specified
	addrString := ""
	if port != "" {
		addrString = fmt.Sprintf("localhost:%s", port)
	} else {
		addrString = fmt.Sprintf("unix://%s", socketPath)
	}

	// Connect to gpuctl to get GPU UUIDs
	conn, err := grpc.NewClient(
		addrString,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("failed to connect to GPU agent: %v", err)
	}
	defer conn.Close()

	client := amdgpu.NewGPUSvcClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.GPUGet(ctx, &amdgpu.GPUGetRequest{})
	if err != nil {
		return fmt.Errorf("GPUGet call failed: %v", err)
	}

	// Create MockInbandError structure with GPU UUIDs and empty afid arrays
	mockError := MockInbandError{
		CPER: make([]MockCPEREntry, 0, len(resp.Response)),
	}

	for _, gpu := range resp.Response {
		// Convert GPU Spec.Id ([]byte) to UUID string
		gpuUUID := uuid.UUID(gpu.Spec.Id).String()
		entry := MockCPEREntry{
			GPU:  gpuUUID,
			AFId: []uint64{}, // Empty array as specified
		}
		mockError.CPER = append(mockError.CPER, entry)
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(mockError, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal mock error data: %v", err)
	}

	// Create directory if it doesn't exist
	dirPath := "/mockdata/inband-ras"
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", dirPath, err)
	}

	// Write to file (override if exists)
	filePath := "/mockdata/inband-ras/error_list"
	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %v", filePath, err)
	}

	fmt.Printf("Successfully created mock inband RAS error_list at %s\n", filePath)
	fmt.Printf("Added %d GPU(s) to the mock error list\n", len(mockError.CPER))
	return nil
}

// Root command
var rootCmd = &cobra.Command{
	Use:   "metricsclient",
	Short: "A CLI tool for querying GPU metrics and managing device resources",
	Long: `metricsclient is a command-line interface for interacting with GPU metrics service,
GPU agents, and Kubernetes resources related to AMD GPUs.`,
	Run: func(cmd *cobra.Command, args []string) {
		// If ecc-file-path is provided, set error instead of listing
		if eccFilePath != "" {
			if err := setError(socketPath, eccFilePath); err != nil {
				log.Fatalf("failed to set error: %v", err)
			}
			return
		}
		// Default behavior: list all GPUs
		if err := send(socketPath); err != nil {
			log.Fatalf("request failed: %v", err)
		}
	},
}

// List command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all GPU states from metrics service",
	Long:  `Query the metrics service and list all GPU states with their health status and associated workloads.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := send(socketPath); err != nil {
			log.Fatalf("request failed: %v", err)
		}
	},
}

// Get command
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Get health status of a specific GPU",
	Long:  `Query the metrics service for the health status of a specific GPU by ID.`,
	Run: func(cmd *cobra.Command, args []string) {
		gpuID, _ := cmd.Flags().GetString("id")
		if err := get(socketPath, gpuID); err != nil {
			log.Fatalf("request failed: %v", err)
		}
	},
}

// GPUctl command
var gpuctlCmd = &cobra.Command{
	Use:   "gpuctl",
	Short: "Query GPU agent directly",
	Long:  `Connect directly to the GPU agent via socket or IP:port and retrieve GPU information.`,
	Run: func(cmd *cobra.Command, args []string) {
		port, _ := cmd.Flags().GetString("port")
		socket, _ := cmd.Flags().GetString("socket")
		jsonOut, _ := cmd.Flags().GetBool("json")

		portStr, socketStr := determineGPUAgentConnection(port, socket)
		getGpuAgent(portStr, socketStr, jsonOut, gpuGetFilterFromFlags(cmd))
	},
}

// gpuGetFilterFromFlags builds a GPUGetFilter from the --skip-* flags, or nil
// when none are set (fetch everything).
func gpuGetFilterFromFlags(cmd *cobra.Command) *amdgpu.GPUGetFilter {
	b := func(name string) bool { v, _ := cmd.Flags().GetBool(name); return v }
	f := &amdgpu.GPUGetFilter{
		SkipClockStatus:    b("skip-clock-status"),
		SkipPCIeStatus:     b("skip-pcie-status"),
		SkipXGMIStatus:     b("skip-xgmi-status"),
		SkipProcessStatus:  b("skip-process-status"),
		SkipUALinkStatus:   b("skip-ualink-status"),
		SkipVRAMUsageStats: b("skip-vram-usage-stats"),
		SkipECCStats:       b("skip-ecc-stats"),
		SkipViolationStats: b("skip-violation-stats"),
		SkipPCIeStats:      b("skip-pcie-stats"),
		SkipXGMIStats:      b("skip-xgmi-stats"),
		SkipActivityStats:  b("skip-activity-stats"),
	}
	if !f.SkipClockStatus && !f.SkipPCIeStatus && !f.SkipXGMIStatus &&
		!f.SkipProcessStatus && !f.SkipUALinkStatus && !f.SkipVRAMUsageStats &&
		!f.SkipECCStats && !f.SkipViolationStats && !f.SkipPCIeStats &&
		!f.SkipXGMIStats && !f.SkipActivityStats {
		return nil
	}
	return f
}

// Device map command
var deviceMapCmd = &cobra.Command{
	Use:   "device-map",
	Short: "Show logical GPU device map",
	Long:  `Display the mapping between render IDs and device names for AMD GPUs.`,
	Run: func(cmd *cobra.Command, args []string) {
		getDeviceMap()
	},
}

// Pod resources command
var podResourcesCmd = &cobra.Command{
	Use:   "pod-resources",
	Short: "Get node resource information",
	Long:  `Query the kubelet for pod resource information including GPU allocations.`,
	Run: func(cmd *cobra.Command, args []string) {
		getPodResources()
	},
}

// Node pods command
var nodePodsCmd = &cobra.Command{
	Use:   "node-pods",
	Short: "Get pod labels from the current node",
	Long:  `List all pods scheduled on the current Kubernetes node with their labels.`,
	Run: func(cmd *cobra.Command, args []string) {
		getNodePods(kubeConfig)
	},
}

// Node labels command
var nodeLabelsCmd = &cobra.Command{
	Use:   "node-labels",
	Short: "Get Kubernetes node labels",
	Long:  `Retrieve and display all labels for the current Kubernetes node.`,
	Run: func(cmd *cobra.Command, args []string) {
		getNodeLabel(kubeConfig)
	},
}

// Setup mock inband RAS command
var setupMockInbandCmd = &cobra.Command{
	Use:   "setup-mock-inbandras",
	Short: "Setup mock inband RAS error_list file",
	Long:  `Create a mock inband RAS error_list file with GPU UUIDs from the GPU agent.`,
	Run: func(cmd *cobra.Command, args []string) {
		port, _ := cmd.Flags().GetString("port")
		socket, _ := cmd.Flags().GetString("socket")

		portStr, socketStr := determineGPUAgentConnection(port, socket)
		if err := setupMockInbandRAS(portStr, socketStr); err != nil {
			log.Fatalf("error setting up mock inband RAS: %v", err)
		}
	},
}

func init() {
	// Global persistent flags
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().StringVar(&socketPath, "socket", fmt.Sprintf("unix://%v", globals.MetricsSocketPath), "Metrics gRPC socket path")
	rootCmd.PersistentFlags().StringVar(&kubeConfig, "kube-config", "", "Kubernetes config file path")
	rootCmd.PersistentFlags().StringVar(&eccFilePath, "ecc-file-path", "", "Path to JSON error configuration file")

	// Get command flags
	getCmd.Flags().String("id", "1", "GPU ID to query")

	// GPUctl command flags
	gpuctlCmd.Flags().String("port", "", "gRPC port for gpuagent (use this for IP:port connection)")
	gpuctlCmd.Flags().String("socket", globals.GPUAgentDefaultSocketPath, "Socket path for gpuagent connection")
	gpuctlCmd.Flags().Bool("json", false, "Output in JSON format")
	// GPUGetFilter skip flags; any set flag makes gpuagent skip that collector.
	gpuctlCmd.Flags().Bool("skip-clock-status", false, "skip clock status")
	gpuctlCmd.Flags().Bool("skip-pcie-status", false, "skip PCIe status")
	gpuctlCmd.Flags().Bool("skip-xgmi-status", false, "skip XGMI error status")
	gpuctlCmd.Flags().Bool("skip-process-status", false, "skip process list")
	gpuctlCmd.Flags().Bool("skip-ualink-status", false, "skip UALink state")
	gpuctlCmd.Flags().Bool("skip-vram-usage-stats", false, "skip VRAM usage")
	gpuctlCmd.Flags().Bool("skip-ecc-stats", false, "skip ECC error counts")
	gpuctlCmd.Flags().Bool("skip-violation-stats", false, "skip violation stats")
	gpuctlCmd.Flags().Bool("skip-pcie-stats", false, "skip PCIe stats")
	gpuctlCmd.Flags().Bool("skip-xgmi-stats", false, "skip XGMI counters")
	gpuctlCmd.Flags().Bool("skip-activity-stats", false, "skip GPU activity and usage")

	// Setup mock inband RAS command flags
	setupMockInbandCmd.Flags().String("port", "", "gRPC port for gpuagent (use this for IP:port connection)")
	setupMockInbandCmd.Flags().String("socket", globals.GPUAgentDefaultSocketPath, "Socket path for gpuagent connection")

	// Add all commands to root
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(gpuctlCmd)
	rootCmd.AddCommand(deviceMapCmd)
	rootCmd.AddCommand(podResourcesCmd)
	rootCmd.AddCommand(nodePodsCmd)
	rootCmd.AddCommand(nodeLabelsCmd)
	rootCmd.AddCommand(setupMockInbandCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
