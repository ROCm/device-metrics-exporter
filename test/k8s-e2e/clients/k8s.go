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
	"time"

	testutils "github.com/ROCm/device-metrics-exporter/test/utils"
	"github.com/prometheus/common/expfmt"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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

// DeleteNamespaceAndWait deletes the namespace (if it exists) and waits until
// it is fully removed from the API server, up to the given timeout.
// labelSelector is unused (reserved for future filtering) and may be "".
// Returns nil if the namespace was not found to begin with, or once it is gone.
func (k *K8sClient) DeleteNamespaceAndWait(ctx context.Context, namespace, _ string, timeout time.Duration) error {
	err := k.client.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{})
	if err != nil {
		// Already gone — not an error.
		if isNotFound(err) {
			return nil
		}
		return fmt.Errorf("delete namespace %s: %w", namespace, err)
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, err := k.client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if isNotFound(err) {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for namespace %s to be deleted", namespace)
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

func (k *K8sClient) GetEndpointByLabel(ctx context.Context, namespace string, labelMap map[string]string) ([]discoveryv1.EndpointSlice, error) {
	list, err := k.client.DiscoveryV1().EndpointSlices(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labelMap).String(),
	})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
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

func (k *K8sClient) GetMetricsFromEp(ctx context.Context, port uint, ep *discoveryv1.EndpointSlice) (payload map[string]*testutils.GPUMetric, err error) {
	for _, endpoint := range ep.Endpoints {
		for _, addr := range endpoint.Addresses {
			resp, err := http.Get(fmt.Sprintf("http://%s:%d/metrics", addr, port))
			if err != nil {
				log.Printf("failed to get metrics from %s:%d/metrics, %v", addr, port, err)
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
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
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

// GetMetricValuesFromPod scrapes /metrics from the exporter running in the pod
// and returns a map of metric name → value for the first GPU found.
func (k *K8sClient) GetMetricValuesFromPod(ctx context.Context, rc *restclient.Config, pod *corev1.Pod) (map[string]float64, error) {
	if pod == nil {
		return nil, fmt.Errorf("invalid pod")
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

	executor, err := remotecommand.NewSPDYExecutor(rc, "POST", req.URL())
	if err != nil {
		return nil, err
	}

	buf := &bytes.Buffer{}
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: buf,
		Tty:    false,
	})
	if err != nil {
		return nil, fmt.Errorf("%w failed executing command %s on %v/%v", err, cmd, pod.Namespace, pod.Name)
	}

	p := expfmt.TextParser{}
	mf, err := p.TextToMetricFamilies(buf)
	if err != nil {
		return nil, fmt.Errorf("%w failed parsing metrics", err)
	}

	values := make(map[string]float64)
	for name, family := range mf {
		if len(family.Metric) == 0 {
			continue
		}
		m := family.Metric[0]
		if g := m.GetGauge(); g != nil {
			values[name] = g.GetValue()
		} else if ct := m.GetCounter(); ct != nil {
			values[name] = ct.GetValue()
		}
	}
	return values, nil
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
	existing, err := k.client.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if existing.Data == nil {
		existing.Data = map[string]string{}
	}
	existing.Data["config.json"] = json
	_, err = k.client.CoreV1().ConfigMaps(namespace).Update(ctx, existing, metav1.UpdateOptions{})
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
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: buf,
		Tty:    false,
	})
	if err != nil {
		return "", fmt.Errorf("failed to run command on pod %v. Error:%v", pod.Name, err)
	}

	return buf.String(), nil
}

// isNotFound returns true when err is a Kubernetes 404 Not Found error.
func isNotFound(err error) bool {
	return err != nil && k8serrors.IsNotFound(err)
}
