# device-metrics-exporter-charts

![Version: v1.3.0](https://img.shields.io/badge/Version-v1.3.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: v1.3.0](https://img.shields.io/badge/AppVersion-v1.3.0-informational?style=flat-square)

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
| image.pullPolicy | string | `"Always"` | metrics exporter image pullPolicy |
| image.pullSecrets | string | `""` | metrics exporter image pullSecret name |
| image.repository | string | `"docker.io/rocm/device-metrics-exporter"` | repository URL for the metrics exporter image |
| image.tag | string | `"v1.3.0"` | metrics exporter image tag |
| image.initContainerImage | string | `"busybox:1.36"` | metrics exporter initContainer image |
| nodeSelector | object | `{}` | Add node selector for the daemonset of metrics exporter |
| platform | string | `"k8s"` |  |
| service.ClusterIP.port | int | `5000` | set port for ClusterIP type service |
| service.NodePort.nodePort | int | `32500` | set nodePort for NodePort type service   |
| service.NodePort.port | int | `5000` | set port for NodePort type service    |
| service.type | string | `"ClusterIP"` | metrics exporter service type, could be ClusterIP or NodePort |
| tolerations | list | `[]` | Add tolerations for deploying metrics exporter on tainted nodes |
| serviceMonitor.enabled | bool | `false` | Create a ServiceMonitor resource for Prometheus Operator |
| serviceMonitor.interval | string | `"30s"` | Scrape interval for the ServiceMonitor|
| serviceMonitor.honorLabels | bool | `true` | Honor labels configuration for ServiceMonitor|
| serviceMonitor.honorTimestamps | bool | `true`| Honor timestamps configuration for ServiceMonitor |
| serviceMonitor.attachMetadata.node | bool | `false` | Add node metadata as labels |
| serviceMonitor.labels | list | `[]` | Additional labels for the ServiceMonitor |
| serviceMonitor.relabelings | list | `[]` | RelabelConfigs to apply to samples before scraping |
| serviceMonitor.metricRelabelings | list | `[]` | Relabeling rules applied to individual scraped metrics |


----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
