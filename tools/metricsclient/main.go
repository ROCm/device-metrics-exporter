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
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/fsysdevice"
	k8sclient "github.com/ROCm/device-metrics-exporter/pkg/client"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/metricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kube "k8s.io/kubelet/pkg/apis/podresources/v1alpha1"
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
	if *jout {
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

func getDeviceMap() {
	devices, err := fsysdevice.FindAMDGPUDevices()
	if err != nil {
		fmt.Printf("device get error : %+v", err)
		return
	}
	fmt.Printf("Logical Device Map \n")
	for k, v := range devices {
		fmt.Printf("GPU ID[%v] -> Device Name [%v]\n", k, v)
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

	found := false
	for _, pod := range resp.PodResources {
		for _, container := range pod.Containers {
			for _, devs := range container.GetDevices() {
				if strings.HasPrefix(devs.ResourceName, globals.AMDGPUResourcePrefix) {
					for _, devId := range devs.DeviceIds {
						fmt.Printf("dev:ns/pod/container [{%v}%v/%v/%v]\n",
							devId, pod.Name, pod.Namespace, container.Name)
						found = true
					}
				}
			}
		}
	}
	if found {
		return
	}
	fmt.Printf("no associations found\n")
	fmt.Printf("pod resp:\n %+v\n", resp)
}

var jout = flag.Bool("json", false, "output in json format")

func main() {
	var (
		socketPath   = flag.String("socket", fmt.Sprintf("unix://%v", globals.MetricsSocketPath), "metrics grpc socket path")
		getOpt       = flag.Bool("get", false, "get health status of gpu")
		setId        = flag.String("id", "1", "gpu id")
		getNodeLabel = flag.Bool("label", false, "get k8s node label")
		podRes       = flag.Bool("pod", false, "get node resource info")
		nodePod      = flag.Bool("npod", false, "get pod labels from node")
		devMap       = flag.Bool("gpu", false, "show logical gpu device map")
		eccFile      = flag.String("ecc-file-path", "", "json ecc err file")
	)
	flag.Parse()

	if *getOpt {
		err := get(*socketPath, *setId)
		if err != nil {
			log.Fatalf("request failed :%v", err)
		}
	} else {
		err := send(*socketPath)
		if err != nil {
			log.Fatalf("request failed :%v", err)
		}
	}

	if *podRes {
		getPodResources()
		return
	}

	if *nodePod {
		nodeName := utils.GetNodeName()
		if nodeName == "" {
			fmt.Println("not a k8s deployment")
			return
		}
		kc, err := k8sclient.NewClient(context.Background(), nodeName)
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
			fmt.Println()
		}
		return
	}

	if *devMap {
		getDeviceMap()
		return
	}

	if *getNodeLabel {
		nodeName := utils.GetNodeName()
		if nodeName == "" {
			fmt.Println("not a k8s deployment")
			return
		}
		kc, err := k8sclient.NewClient(context.Background(), nodeName)
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

	if *eccFile != "" {
		if err := setError(*socketPath, *eccFile); err != nil {
			fmt.Printf("err: %+v", err)
			return
		}
	}
}
