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

package clients

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"

	testutils "github.com/ROCm/device-metrics-exporter/test/utils"
	"github.com/prometheus/common/expfmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/scheme"
)

type K8sClient struct {
	client *kubernetes.Clientset
}

func NewK8sClient(config *restclient.Config) (*K8sClient, error) {
	k8sc := K8sClient{}
	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	k8sc.client = cs
	return &k8sc, nil
}

func (k *K8sClient) CreateNamespace(ctx context.Context, namespace string) error {
	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
		Status: corev1.NamespaceStatus{},
	}
	_, err := k.client.CoreV1().Namespaces().Create(ctx, namespaceObj, metav1.CreateOptions{})
	return err
}

func (k *K8sClient) DeleteNamespace(ctx context.Context, namespace string) error {
	return k.client.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{})
}

func (k *K8sClient) GetPodsByLabel(ctx context.Context, namespace string, labelMap map[string]string) ([]corev1.Pod, error) {
	podList, err := k.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labelMap).String(),
	})
	if err != nil {
		return nil, err
	}
	return podList.Items, nil
}

func (k *K8sClient) GetNodesByLabel(ctx context.Context, labelMap map[string]string) ([]corev1.Node, error) {
	nodeList, err := k.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labelMap).String(),
	})
	if err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

func (k *K8sClient) GetServiceByLabel(ctx context.Context, namespace string, labelMap map[string]string) ([]corev1.Service, error) {
	nodeList, err := k.client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labelMap).String(),
	})
	if err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

func (k *K8sClient) GetEndpointByLabel(ctx context.Context, namespace string, labelMap map[string]string) ([]corev1.Endpoints, error) {
	nodeList, err := k.client.CoreV1().Endpoints(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labelMap).String(),
	})
	if err != nil {
		return nil, err
	}
	return nodeList.Items, nil
}

func (k *K8sClient) ValidatePod(ctx context.Context, namespace, podName string) error {
	pod, err := k.client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("unexpected error getting pod %s; err: %w", podName, err)
	}

	for _, c := range pod.Status.ContainerStatuses {
		if c.State.Waiting != nil && c.State.Waiting.Reason == "CrashLoopBackOff" {
			return fmt.Errorf("pod %s in namespace %s is in CrashLoopBackOff", pod.Name, pod.Namespace)
		}
	}

	return nil
}

func (k *K8sClient) GetMetricsFromEp(ctx context.Context, port uint, ep *corev1.Endpoints) (payload map[string]*testutils.GPUMetric, err error) {
	for _, subnet := range ep.Subsets {
		for _, addr := range subnet.Addresses {
			resp, err := http.Get(fmt.Sprintf("http://%v:%d/metrics", addr, port))
			if err != nil {
				log.Printf("failed to get metrics from %v:%d/metrics, %v", addr, port, err)
				continue
			}
			defer resp.Body.Close()
			bodyBytes, err := io.ReadAll(resp.Body)
			if err != nil {
				continue
			}
			payload, err = testutils.ParsePrometheusMetrics(string(bodyBytes))
			if err != nil {
				continue
			}
			return payload, err
		}
	}
	return nil, fmt.Errorf("ep invalid status or no ip present")
}

func (k *K8sClient) GetMetricsCmdFromPod(ctx context.Context, rc *restclient.Config, pod *corev1.Pod) (labels []string, fields []string, err error) {
	if pod == nil {
		return nil, nil, fmt.Errorf("invalid pod")
	}
	req := k.client.CoreV1().RESTClient().Post().Resource("pods").Name(pod.Name).
		Namespace(pod.Namespace).
		SubResource("exec")

	cmd := "curl -s localhost:5000/metrics"
	req.VersionedParams(&corev1.PodExecOptions{
		Command: []string{"/bin/sh", "-c", cmd},
		Stdin:   false,
		Stdout:  true,
		Stderr:  false,
		TTY:     false,
	}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(rc, "POST", req.URL())
	if err != nil {
		return nil, nil, err
	}

	buf := &bytes.Buffer{}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: buf,
		Tty:    false,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("%w failed executing command %s on %v/%v", err, cmd, pod.Namespace, pod.Name)
	}
	//log.Printf("\nbuf : %v\n", buf.String())
	p := expfmt.TextParser{}
	m, err := p.TextToMetricFamilies(buf)
	if err != nil {
		return nil, nil, fmt.Errorf("%w failed parsing to metrics", err)
	}
	for _, f := range m {
		fields = append(fields, *f.Name)
		for _, km := range f.Metric {
			if len(labels) != 0 {
				continue
			}
			for _, lp := range km.GetLabel() {
				labels = append(labels, *lp.Name)
			}
		}

	}
	return
}

func (k *K8sClient) CreateConfigMap(ctx context.Context, namespace string, name string, json string) error {
	mcfgMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			"config.json": json,
		},
	}

	_, err := k.client.CoreV1().ConfigMaps(namespace).Create(ctx, mcfgMap, metav1.CreateOptions{})
	return err
}

func (k *K8sClient) UpdateConfigMap(ctx context.Context, namespace string, name string, json string) error {
	mcfgMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			"config.json": json,
		},
	}

	_, err := k.client.CoreV1().ConfigMaps(namespace).Update(ctx, mcfgMap, metav1.UpdateOptions{})
	return err
}

func (k *K8sClient) DeleteConfigMap(ctx context.Context, namespace string, name string) error {
	return k.client.CoreV1().ConfigMaps(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func (k *K8sClient) ExecCmdOnPod(ctx context.Context, rc *restclient.Config, pod *corev1.Pod, container, execCmd string) (string, error) {
	if pod == nil {
		return "", fmt.Errorf("No pod specified")
	}
	req := k.client.CoreV1().RESTClient().Post().Resource("pods").Name(pod.Name).Namespace(pod.Namespace).SubResource("exec")
	req.VersionedParams(&corev1.PodExecOptions{
		Container: container,
		Command:   []string{"/bin/sh", "-c", execCmd},
		Stdin:     false,
		Stdout:    true,
		Stderr:    false,
		TTY:       false,
	}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(rc, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("failed to create command executor. Error:%v", err)
	}
	buf := &bytes.Buffer{}
	err = executor.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdout: buf,
		Tty:    false,
	})
	if err != nil {
		return "", fmt.Errorf("failed to run command on pod %v. Error:%v", pod.Name, err)
	}

	return buf.String(), nil
}
