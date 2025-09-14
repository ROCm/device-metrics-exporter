# device-metrics-exporter-charts

![Version: v1.4.0](https://img.shields.io/badge/Version-v1.4.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v1.4.0](https://img.shields.io/badge/AppVersion-v1.4.0-informational?style=flat-square)

A Helm chart for AMD Device Metric Exporter

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| Praveen Kumar Shanmugam | <prshanmug@amd.com> |  |
| Yan Sun | <yan.sun3@amd.com> |  |
| Shrey Ajmera | <shrey.ajmera@amd.com> |  |

## Requirements

Kubernetes: `>= 1.29.0-0`

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| configMap | string | `""` | configMap name for the customizing configs and mount into metrics exporter container |
| image.initContainerImage | string | `"busybox:1.36"` | metrics exporter initContainer image |
| image.pullPolicy | string | `"Always"` | metrics exporter image pullPolicy |
| image.pullSecrets | string | `""` | metrics exporter image pullSecret name |
| image.repository | string | `"docker.io/rocm/device-metrics-exporter"` | repository URL for the metrics exporter image |
| image.tag | string | `"v1.4.0"` | metrics exporter image tag |
| kubelet | object | `{"podResourceAPISocketPath":"/var/lib/kubelet/pod-resources"}` | kubelet configuration |
| kubelet.podResourceAPISocketPath | string | `"/var/lib/kubelet/pod-resources"` | host path for kubelet pod-resources directory (optional)    - vanilla k8s kubelet path: /var/lib/kubelet/pod-resources    - micro k8s kubelet path: /var/snap/microk8s/common/var/lib/kubelet/pod-resources/    - default to /var/lib/kubelet/pod-resources |
| nodeSelector | object | `{}` | Add node selector for the daemonset of metrics exporter |
| platform | string | `"k8s"` | Specify the platform to deploy the metrics exporter, k8s or openshift |
| service.ClusterIP.port | int | `5000` | set port for ClusterIP type service |
| service.NodePort.nodePort | int | `32500` | set nodePort for NodePort type service   |
| service.NodePort.port | int | `5000` | set port for NodePort type service    |
| service.type | string | `"ClusterIP"` | metrics exporter service type, could be ClusterIP or NodePort |
| serviceMonitor | object | `{"attachMetadata":{"node":false},"enabled":false,"honorLabels":true,"honorTimestamps":true,"interval":"30s","labels":{},"metricRelabelings":[],"relabelings":[]}` | ServiceMonitor configuration |
| serviceMonitor.attachMetadata | object | `{"node":false}` | Adds node metadata to discovered targets for node-based filtering |
| serviceMonitor.enabled | bool | `false` | Whether to create a ServiceMonitor resource for Prometheus Operator |
| serviceMonitor.honorLabels | bool | `true` | Honor labels configuration for ServiceMonitor |
| serviceMonitor.honorTimestamps | bool | `true` | Honor timestamps configuration for ServiceMonitor |
| serviceMonitor.interval | string | `"30s"` | Scrape interval for the ServiceMonitor |
| serviceMonitor.labels | object | `{}` | Additional labels for the ServiceMonitor |
| serviceMonitor.metricRelabelings | list | `[]` | Relabeling rules applied to individual scraped metrics |
| serviceMonitor.relabelings | list | `[]` | RelabelConfigs to apply to samples before scraping |
| tolerations | list | `[]` | Add tolerations for deploying metrics exporter on tainted nodes |

