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
	"errors"
	"fmt"
	"log"
	"reflect"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
	//
	// Uncomment to load all auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth"
	//
	// Or uncomment to load specific auth plugins
	// _ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

// PodUniqueKey - key for uniquely identifying pod
type PodUniqueKey struct {
	PodName   string
	Namespace string
}

func (p *PodUniqueKey) String() string {
	return fmt.Sprintf("%v-%v", p.Namespace, p.PodName)
}

type K8sClient struct {
	sync.Mutex
	ctx          context.Context
	clientset    kubernetes.Interface
	nodeName     string
	stopCh       chan struct{}
	started      bool
	nodeInformer cache.SharedIndexInformer
	podInformer  cache.SharedIndexInformer
}

func NewClient(ctx context.Context, nodeName string) (*K8sClient, error) {

	if nodeName == "" {
		return nil, fmt.Errorf("node name cannot be empty")
	}
	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Log.Printf("k8s cluster config error %v", err)
		return nil, err
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Log.Printf("clientset from config failed %v", err)
		return nil, err
	}

	k8c := &K8sClient{
		ctx:       ctx,
		clientset: clientset,
		nodeName:  nodeName,
		stopCh:    make(chan struct{}),
		started:   false,
	}
	return k8c, nil
}

func (k *K8sClient) CreateEvent(evtObj *v1.Event) error {
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

func (k *K8sClient) AddNodeLabel(nodeName string, keys []string, val string) error {
	k.Lock()
	defer k.Unlock()
	ctx, cancel := context.WithCancel(k.ctx)
	defer cancel()

	patch := []map[string]interface{}{}
	for _, key := range keys {
		patch = append(patch, map[string]interface{}{
			"op":    "add",
			"path":  fmt.Sprintf("/metadata/labels/%v", key),
			"value": val,
		})
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch %v: %v", patch, err)
	}
	_, err = k.clientset.CoreV1().Nodes().Patch(ctx, nodeName, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		logger.Log.Printf("failed to add label %+v to node %+v err %+v", keys, nodeName, err)
	}
	return err
}

func (k *K8sClient) RemoveNodeLabel(nodeName string, keys []string) error {
	k.Lock()
	defer k.Unlock()
	ctx, cancel := context.WithCancel(k.ctx)
	defer cancel()
	patch := []map[string]interface{}{}
	for _, key := range keys {
		patch = append(patch, map[string]interface{}{
			"op":   "remove",
			"path": fmt.Sprintf("/metadata/labels/%v", key),
		})
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch %v: %v", patch, err)
	}
	_, err = k.clientset.CoreV1().Nodes().Patch(ctx, nodeName, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		logger.Log.Printf("failed to remove label %+v from node %+v err %+v", keys, nodeName, err)
	}
	return err
}

func (k *K8sClient) UpdateHealthLabel(nodeName string, newHealthMap map[string]string) error {
	k.Lock()
	defer k.Unlock()

	ctx, cancel := context.WithCancel(k.ctx)
	defer cancel()

	node, err := k.clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		logger.Log.Printf("k8s internal node get failed %v", err)
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
		logger.Log.Printf("k8s internal node update failed %v", err)
		return err
	}

	return nil
}

// Watch starts the label watchers with reconnection support
func (k *K8sClient) Watch() error {
	k.Lock()
	if k.started {
		k.Unlock()
		return errors.New("watcher already started")
	}
	k.started = true
	k.Unlock()

	go k.runWithReconnect()
	return nil
}

func (k *K8sClient) runWithReconnect() {
	retryInterval := 5 * time.Second
	for {
		if err := k.startWatchers(); err != nil {
			logger.Log.Printf("Watcher error: %v. Retrying in %s...\n", err, retryInterval)
		} else {
			logger.Log.Printf("Watchers stopped. Restarting...")
		}

		select {
		case <-time.After(retryInterval):
			continue
		case <-k.stopCh:
			return
		}
	}
}

func (k *K8sClient) startWatchers() error {
	nodeFactory := informers.NewSharedInformerFactoryWithOptions(
		k.clientset,
		0,
		informers.WithTweakListOptions(func(opt *metav1.ListOptions) {
			opt.FieldSelector = fields.OneTermEqualSelector("metadata.name", k.nodeName).String()
		}),
	)
	podFactory := informers.NewSharedInformerFactoryWithOptions(
		k.clientset,
		0,
		informers.WithTweakListOptions(func(opt *metav1.ListOptions) {
			opt.FieldSelector = fields.OneTermEqualSelector("spec.nodeName", k.nodeName).String()
		}),
	)

	k.nodeInformer = nodeFactory.Core().V1().Nodes().Informer()
	k.podInformer = podFactory.Core().V1().Pods().Informer()

	k.nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if node, ok := obj.(*v1.Node); ok {
				logger.Log.Printf("node added with labels: %+v", node.Labels)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldNode := oldObj.(*v1.Node)
			newNode := newObj.(*v1.Node)
			if !reflect.DeepEqual(oldNode.Labels, newNode.Labels) {
				logger.Log.Printf("node updated with labels: %+v", newNode.Labels)
			}
		},
	})
	k.podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if pod, ok := obj.(*v1.Pod); ok {
				logger.Log.Printf("pod[%v-%v] added with labels: %+v",
					pod.Name, pod.Namespace, pod.Labels)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldPod := oldObj.(*v1.Pod)
			newPod := newObj.(*v1.Pod)
			if !reflect.DeepEqual(oldPod.Labels, newPod.Labels) {
				logger.Log.Printf("pod[%v-%v] updated with labels: %+v",
					newPod.Name, newPod.Namespace, newPod.Labels)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if pod, ok := obj.(*v1.Pod); ok {
				logger.Log.Printf("pod[%v-%v] deleted", pod.Name, pod.Namespace)
			}
		},
	})

	// Start and block until synced
	stopCh := make(chan struct{})
	defer close(stopCh)

	go k.nodeInformer.Run(stopCh)
	go k.podInformer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, k.nodeInformer.HasSynced, k.podInformer.HasSynced) {
		return errors.New("cache sync failed")
	}

	// Block until stop signal received
	select {
	case <-k.stopCh:
		return nil
	}
}

func (k *K8sClient) Stop() {
	close(k.stopCh)
}

func (k *K8sClient) GetClientSet() kubernetes.Interface {
	return k.clientset
}

func (k *K8sClient) GetNodeLabel() (string, error) {
	node, err := k.GetNode()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%+v", node.Labels), nil
}

func (k *K8sClient) GetAllPods() (map[string]map[string]string, error) {
	// Initialize the resulting map
	k8PodLabelsMap := make(map[string]map[string]string)

	pods, err := k.ListPods()
	if err != nil {
		log.Printf("Error fetching pods for node %v: %v", k.nodeName, err)
		return nil, err
	}

	// Process each pod and populate the map
	for _, pod := range pods {
		podKey := PodUniqueKey{
			PodName:   pod.Name,
			Namespace: pod.Namespace,
		}
		k8PodLabelsMap[podKey.String()] = pod.Labels
	}
	return k8PodLabelsMap, nil
}

func (k *K8sClient) GetNode() (*v1.Node, error) {
	if k.nodeInformer == nil || !k.nodeInformer.HasSynced() {
		return nil, errors.New("cache not synced or API server unavailable")
	}
	// since we are watching only self node, we can safely assume the first
	// object in the store is the node we are interested in
	objs := k.nodeInformer.GetStore().List()
	if len(objs) == 0 {
		return nil, errors.New("node not available in cache")
	}
	if node, ok := objs[0].(*v1.Node); ok {
		return node.DeepCopy(), nil
	}
	return nil, errors.New("failed to cast object to *v1.Node")
}

func (k *K8sClient) ListPods() ([]*v1.Pod, error) {
	if k.podInformer == nil || !k.podInformer.HasSynced() {
		return nil, errors.New("cache not synced or API server unavailable")
	}
	pods := []*v1.Pod{}
	for _, obj := range k.podInformer.GetStore().List() {
		if pod, ok := obj.(*v1.Pod); ok {
			pods = append(pods, pod.DeepCopy())
		}
	}
	return pods, nil
}
