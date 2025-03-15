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

package k8sclient

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
	//
	// Uncomment to load all auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth"
	//
	// Or uncomment to load specific auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

type K8sClient struct {
	sync.Mutex
	ctx       context.Context
	clientset *kubernetes.Clientset
}

func NewClient(ctx context.Context) *K8sClient {
	return &K8sClient{
		ctx: ctx,
	}
}

func (k *K8sClient) init() error {
	k.Lock()
	defer k.Unlock()

	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Log.Printf("k8s cluster config error %v", err)
		return err
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Log.Printf("clientset from config failed %v", err)
		return err
	}

	k.clientset = clientset
	return nil
}

func (k *K8sClient) reConnect() error {
	if k.clientset == nil {
		return k.init()
	}
	return nil
}

func (k *K8sClient) CreateEvent(evtObj *v1.Event) error {
	k.reConnect()
	k.Lock()
	defer k.Unlock()
	ctx, cancel := context.WithCancel(k.ctx)
	defer cancel()

	if evtObj == nil {
		logger.Log.Printf("k8s client got empty event object, skip genreating k8s event")
		return fmt.Errorf("k8s client received empty event object")
	}

	if _, err := k.clientset.CoreV1().Events(evtObj.Namespace).Create(ctx, evtObj, metav1.CreateOptions{}); err != nil {
		logger.Log.Printf("failed to generate event %+v, err: %+v", evtObj, err)
		return err
	}

	return nil
}

func (k *K8sClient) GetNodelLabel(nodeName string) (string, error) {
	k.reConnect()
	k.Lock()
	defer k.Unlock()
	ctx, cancel := context.WithCancel(k.ctx)
	defer cancel()

	node, err := k.clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		logger.Log.Printf("k8s internal node get failed %v", err)
		k.clientset = nil
		return "", err
	}
	return fmt.Sprintf("%+v", node.Labels), nil
}

func (k *K8sClient) AddNodeLabel(nodeName, key, val string) error {
	k.reConnect()
	k.Lock()
	defer k.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	patch := []map[string]interface{}{
		{
			"op":    "add",
			"path":  fmt.Sprintf("/metadata/labels/%v", key),
			"value": val,
		},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch %v: %v", patch, err)
	}
	_, err = k.clientset.CoreV1().Nodes().Patch(ctx, nodeName, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		logger.Log.Printf("failed to add label %+v to node %+v err %+v", key, nodeName, err)
	}
	return err
}

func (k *K8sClient) RemoveNodeLabel(nodeName, key string) error {
	k.reConnect()
	k.Lock()
	defer k.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	patch := []map[string]interface{}{
		{
			"op":   "remove",
			"path": fmt.Sprintf("/metadata/labels/%v", key),
		},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch %v: %v", patch, err)
	}
	_, err = k.clientset.CoreV1().Nodes().Patch(ctx, nodeName, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		logger.Log.Printf("failed to remove label %+v from node %+v err %+v", key, nodeName, err)
	}
	return err
}

func (k *K8sClient) UpdateHealthLabel(nodeName string, newHealthMap map[string]string) error {
	k.reConnect()
	k.Lock()
	defer k.Unlock()

	ctx, cancel := context.WithCancel(k.ctx)
	defer cancel()

	node, err := k.clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		logger.Log.Printf("k8s internal node get failed %v", err)
		k.clientset = nil
		return err
	}

	oldHealthMap := utils.ParseNodeHealthLabel(node.Labels)

	// check diff
	if reflect.DeepEqual(oldHealthMap, newHealthMap) {
		// logger.Log.Printf("ignoring update no change on label values")
		return nil
	}
	utils.RemoveNodeHealthLabel(node.Labels)
	utils.AddNodeHealthLabel(node.Labels, newHealthMap)

	// Update the node
	_, err = k.clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		//TODO : disable for azure image drop
		//logger.Log.Printf("k8s internal node update failed %v", err)
		k.clientset = nil
		return err
	}

	return nil
}
