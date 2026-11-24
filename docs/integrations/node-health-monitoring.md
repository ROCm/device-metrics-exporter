# Node health monitoring

Device Metrics Exporter integrates with node health monitoring tools and daemons. It provides the `amdgpuhealth` command-line tool, which monitors specific metrics or in-band RAS errors and checks whether their values are within a defined threshold.

## amdgpuhealth tool

Use the tool to monitor AMD GPU node health in Kubernetes and standalone Debian deployments. The `amdgpuhealth` CLI monitors counter metrics, gauge metrics, and in-band RAS events. You can use it as a standalone tool or integrate it with Node Problem Detector (NPD) in Kubernetes environments.

## amdgpuhealth tool usage guide

Specify the metric to query and a threshold above which the GPU is treated as unhealthy. Use the tool's output and exit code to determine whether a metric or in-band RAS error count has crossed its threshold. If you don't provide a threshold, the default is 0.

- Exit code 0: The metric or error count is within the threshold. The GPU and node are healthy.
- Exit code 1: The metric or error count crossed the threshold. The node is unhealthy.
- Exit code 2: The query for the metric or in-band RAS error failed.

```bash
./amdgpuhealth query counter-metric -m <metric_name> -t <threshold_value>

./amdgpuhealth query gauge-metric -m <metric_name> -t <threshold_value>

./amdgpuhealth query inband-ras-errors --afid <AFID> -s <severity> -t <threshold_value>
```

## Integration with Debian deployments

The `amdgpuhealth` tool is included in the standalone Device Metrics Exporter Debian and RPM packages. Run it directly in a terminal or from a shell script. For example, a script that a cron job runs at regular intervals can monitor several critical metrics and in-band RAS errors.

On Debian installations, the TLS, bearer token, `--duration`, and `--prometheus-endpoint` options are not available.

## Integration with Kubernetes deployment using Node Problem Detector

Node Problem Detector (NPD) makes node problems visible to the upstream layers of the cluster management stack. It runs as a daemon on each node, detects node problems, and reports them to the API server. You can extend NPD to detect AMD GPU problems.

## Node Problem Detector installation

Many Kubernetes clusters like GKE, AKS, etc. come with NPD enabled by default. If it isn't present, install it with its Helm chart. See the [Node Problem Detector installation guide](https://github.com/kubernetes/node-problem-detector?tab=readme-ov-file#installation).

## Custom plugin monitor

The custom plugin monitor is a plugin mechanism for NPD. It lets NPD run monitor scripts written in any language. The monitor scripts must follow the plugin protocol for exit codes and standard output. For details, see the [NPD plugin interface](https://docs.google.com/document/d/1jK_5YloSYtboj-DtfjmYKxfNnUxCAvohLnsH5aGCAYQ/edit#).

Exit codes 0, 1, and 2 are used for plugin monitor. Exit code 0 is treated as working state. Exit code 1 is treated as problem state. Exit code 2 is used for any unknown error. When plugin monitor detects exit code 1, it sets NodeCondition based on the rules defined in the custom plugin monitor config file.

## Node Problem Detector integration

The `amdgpuhealth` utility queries AMD GPU metrics from `device-metrics-exporter` and `Prometheus` endpoint. Based on user-configured thresholds, it determines whether any AMD GPU is in a problem state. NPD custom plugin monitor can invoke this program at configurable intervals to monitor various metrics and assess overall health of AMD GPUs.

The utility `amdgpuhealth` is packaged with the device-metrics-exporter distribution. For **Kubernetes** deployments, the tool is included in the container image and is made available on the node under `/var/lib/amd-metrics-exporter`; NPD must mount that host path so the custom plugin monitor can execute `amdgpuhealth`. For **Debian** installations from the standalone package, `amdgpuhealth` is installed to `/usr/local/bin` (on the default search path for shells and scripts).

Example usage of amdgpuhealth CLI:

```bash
./amdgpuhealth query counter-metric -m <metric_name> -t <threshold_value>

./amdgpuhealth query gauge-metric -m <metric_name> -d <duration> -t <threshold_value>

./amdgpuhealth query inband-ras-errors --afid <AFID> -s <severity> -t <threshold_value>
```

In the above examples, the program queries either a counter or gauge metric. You can define a threshold for each metric. If the reported AMD GPU metric value exceeds the threshold, `amdgpuhealth` prints an error message to standard output and exits with code 1. The NPD plugin uses this exit code and output to update the node condition's status and message respectively, indicating problem with AMD GPU.

Example custom plugin monitor config:

```json
{
  "plugin": "custom",
  "pluginConfig": {
    "invoke_interval": "30s",
    "timeout": "15s",
    "max_output_length": 80,
    "concurrency": 3,
    "enable_message_change_based_condition_update": false
  },
  "source": "amdgpu-custom-plugin-monitor",
  "metricsReporting": true,
  "conditions": [
    {
      "type": "AMDGPUProblem",
      "reason": "AMDGPUIsUp",
      "message": "AMD GPU is up"
    }
  ],
  "rules": [
    {
      "type": "permanent",
      "condition": "AMDGPUProblem",
      "reason": "AMDGPUIsDown",
      "path": "./config/amdgpuhealth",
      "args": [
        "query",
        "counter-metric",
        "-m=GPU_ECC_UNCORRECT_UMC",
        "-t=1"
      ],
      "timeout": "10s"
    },
    {
      "type": "permanent",
      "condition": "AMDGPUProblem",
      "reason": "AMDGPUIsDown",
      "path": "./config/amdgpuhealth",
      "args": [
        "query",
        "gauge-metric",
        "-m=GPUMetricField_GPU_EDGE_TEMPERATURE",
        "-t=100",
        "-d=1h",
        "--prometheus-endpoint=http://localhost:9090"
      ],
      "timeout": "10s"
    }
  ]
}
```

The above NPD config rule #1 queries counter metric `GPU_ECC_UNCORRECT_UMC` value every 30 seconds. If the value crosses the threshold (set to 1), NPD marks the node condition as True.

To query the average value of a gauge metric over a period of time, set up Prometheus to scrape metrics from `amd-device-metrics-exporter` endpoint and store them. Rule #2 in above config queries gauge metric `GPUMetricField_GPU_EDGE_TEMPERATURE` value from the Prometheus server. It queries average temperature for the last 1 hour from the Prometheus endpoint mentioned in CLI argument.

## Handling authorization tokens

If your AMD Device Metrics Exporter or Prometheus endpoints require token-based authorization, NPD must include the token as an HTTP header in its requests. Authorization tokens are sensitive, so store them securely. We recommend using **Kubernetes Secrets** to store the token information and mount them as volumes in the NPD pod.

### Creating an authorization token Secret for the exporter endpoint

You can create a Kubernetes Secret to store the token for the AMD Device Metrics Exporter endpoint in two ways:

#### From a file

```bash
kubectl create secret generic -n <NPD_NAMESPACE> amd-exporter-auth-token --from-file=token=<path-to-token-file>
```

#### From a string literal

```bash
kubectl create secret generic -n <NPD_NAMESPACE> amd-exporter-auth-token --from-literal=token=<your-auth-token>
```

Mount this secret as a volume in your NPD deployment yaml. The same path must be specified as CLI argument in the NPD custom plugin monitor config yaml.

```json
"rules": [
    {
      "type": "permanent",
      "condition": "AMDGPUProblem",
      "reason": "AMDGPUIsDown",
      "path": "./config/amdgpuhealth",
      "args": [
        "query",
        "counter-metric",
        "-m=GPU_ECC_UNCORRECT_UMC",
        "-t=1",
        "--exporter-bearer-token=<token-mount-path>"
      ],
      "timeout": "10s"
    }
]
```

### Creating an authorization token Secret for the Prometheus endpoint

Create a Secret for the Prometheus endpoint the same way. Gauge metric queries need it.

#### From a file

```bash
kubectl create secret generic -n <NPD_NAMESPACE> prometheus-auth-token --from-file=token=<path-to-token-file>
```

#### From a string literal

```bash
kubectl create secret generic -n <NPD_NAMESPACE> prometheus-auth-token --from-literal=token=<your-auth-token>
```

Mount this secret as a volume in your NPD deployment yaml. Pass the mount path in NPD custom plugin monitor json as CLI argument.

```json
"rules": [
    {
      "type": "permanent",
      "condition": "AMDGPUProblem",
      "reason": "AMDGPUIsDown",
      "path": "./config/amdgpuhealth",
      "args": [
        "query",
        "gauge-metric",
        "-m=GPUMetricField_GPU_EDGE_TEMPERATURE",
        "-t=100",
        "--prometheus-bearer-token=<token-mount-path>"
      ],
      "timeout": "10s"
    }
]
```

## Handling mutual TLS (mTLS) authentication

If your AMD Device Metrics Exporter or Prometheus endpoints require TLS/mTLS, NPD must have the necessary certificates to be able to communicate with the endpoints.

For TLS, NPD needs to have server endpoint's Root CA certificate to authenticate the server's certificate. Root CA certificate must be stored as Kubernetes Secrets and mounted as volumes in the NPD pod.

### Creating a Secret for the exporter endpoint root CA

Set the key in the Secret to `ca.crt`.

```bash
kubectl create secret generic -n <NPD_NAMESPACE> amd-exporter-rootca --from-file=ca.crt=<path-to-ca-cert>
```

Mount this secret as a volume in your NPD deployment yaml. Same mount path needs to be passed as CLI argument. Example:

```json
"rules": [
    {
      "type": "permanent",
      "condition": "AMDGPUProblem",
      "reason": "AMDGPUIsDown",
      "path": "./config/amdgpuhealth",
      "args": [
        "query",
        "counter-metric",
        "-m=GPU_ECC_UNCORRECT_UMC",
        "-t=1",
        "--exporter-root-ca=<rootca-mount-path>"
      ],
      "timeout": "10s"
    }
]
```

### Creating a Secret for the Prometheus endpoint root CA

```bash
kubectl create secret generic -n <NPD_NAMESPACE> prometheus-rootca --from-file=ca.crt=<path-to-ca-cert>
```

Mount this secret as a volume in your NPD deployment yaml. Pass the mount path in CLI argument followed by the key `ca.crt`. Example below:

```json
"rules": [
    {
      "type": "permanent",
      "condition": "AMDGPUProblem",
      "reason": "AMDGPUIsDown",
      "path": "./config/amdgpuhealth",
      "args": [
        "query",
        "gauge-metric",
        "-m=GPUMetricField_GPU_EDGE_TEMPERATURE",
        "-t=100",
        "--prometheus-root-ca=<rootca-mount-path>/ca.crt"
      ],
      "timeout": "10s"
    }
]
```

For mTLS, NPD needs a certificate and its corresponding private key. Certificate information can be stored as Kubernetes TLS Secret and mounted as volume in the NPD pod.

### Creating a Secret for the NPD identity certificate

`kubectl create secret tls` stores the certificate and key under the `tls.crt` and `tls.key` keys.

```bash
kubectl create secret tls -n <NPD_NAMESPACE> npd-identity --cert=<path-to-your-certificate> --key=<path-to-your-private-key>
```

Mount the secret as a volume in your NPD deployment yaml. Pass the mount path as CLI argument. Example below:

```json
"rules": [
    {
      "type": "permanent",
      "condition": "AMDGPUProblem",
      "reason": "AMDGPUIsDown",
      "path": "./config/amdgpuhealth",
      "args": [
        "query",
        "counter-metric",
        "-m=GPU_ECC_UNCORRECT_UMC",
        "-t=1",
        "--exporter-root-ca=<rootca-mount-path>",
        "--client-cert=<client-cert-mount-path>"
      ],
      "timeout": "10s"
    },
    {
      "type": "permanent",
      "condition": "AMDGPUProblem",
      "reason": "AMDGPUIsDown",
      "path": "./config/amdgpuhealth",
      "args": [
        "query",
        "gauge-metric",
        "-m=GPUMetricField_GPU_EDGE_TEMPERATURE",
        "-t=100",
        "--prometheus-root-ca=<rootca-mount-path>/ca.crt",
        "--client-cert=<client-cert-mount-path>"
      ],
      "timeout": "10s"
    }
]
```
