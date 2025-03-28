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
	"time"

	k8sclient "github.com/ROCm/device-metrics-exporter/pkg/client"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/metricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
	kube "k8s.io/kubelet/pkg/apis/podresources/v1alpha1"
)

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
	fmt.Println("ID\tHealth\tAssociated Workload\t")
	fmt.Println("------------------------------------------------")
	for i := 0; i < len(sortOp); i++ {
		gs := sortOp[fmt.Sprintf("%d", i)]
		fmt.Printf("%v\t%v\t%+v\t\r\n", gs.ID, gs.Health, gs.AssociatedWorkload)
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
				if devs.ResourceName == globals.AMDGPUResourceLabel {
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

	if *getNodeLabel {
		nodeName := os.Getenv("NODE_NAME")
		if nodeName == "" {
			fmt.Println("not a k8s deployment")
			return
		}
		kc := k8sclient.NewClient(context.Background())
		labels, err := kc.GetNodelLabel(nodeName)
		if err != nil {
			fmt.Printf("err: %+v", err)
			return
		}
		fmt.Printf("node[%v] labels[%+v]", nodeName, labels)
	}

	if *eccFile != "" {
		if err := setError(*socketPath, *eccFile); err != nil {
			fmt.Printf("err: %+v", err)
			return
		}
	}
}
