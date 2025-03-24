# Device Metrics Exporter

Device Metrics Exporter exports metrics from AMD GPUs to collectors like Prometheus.

## Supported Platforms
  - Ubuntu 22.04

## RDC version
  - ROCM 6.2

## Build and Run Instructions

### Helm Charts Deployment for Kubernetes
- Prerequisites
  - Kubernetes cluster is up and running
  - Helm tool is installed on the node with kubectl + kube config file to get access to the cluster
- Installation
  - Download the device metrics exporter helm charts .tgz file
  - Prepare ```values.yaml``` to setup the deployment parameters, for example:
    ```yaml
    platform: k8s
    nodeSelector: {} # add customized nodeSelector for metrics exporter daemonset
    image:
      repository: docker.io/rocm/device-metrics-exporter
      tag: v1.0.0
      pullPolicy: Always
      pullSecrets: "" # put name of docker-registry secret here if needed for exporter image
    service:
      type: NodePort # select NodePort or ClusterIP as metrics exporter's service type
      ClusterIP:
        port: 5000 # cluster internal service port
      NodePort:      
        port: 5000 # cluster internal service port
        nodePort: 32500 # external node port 
    configMap: "" # put name of configmap here if needed for customizing exported stats
    ```
  - (Optional) if you want to customize the exported stats, please create a configmap by using ```example/configmap.yaml``` (please modify the namespace to align with helm install command), and put the configmap name into ```values.yaml```.
  - Run ```helm install``` command to deploy exporter in your Kubernetes cluster:
    ```helm install exporter ./amdgpu-metrics-exporter-charts-v1.0.0.tgz -n mynamespace -f values.yaml```
- Update config:
  - Option 1: you can directly modify the Kubernetes resource to modify the config, including modifying configmap, service, rbac or daemonset resources.
  - Option 2: you can prepare the updated ```values.yaml``` and do a helm chart upgrade: ```helm upgrade exporter -n mynamespace -f updated_values.yaml```
- Uninstallation
  - Uninstall the helm charts by running: 
    ```helm uninstall exporter -n mynamespace```

### Run prometheus (Testing)
   ```
	docker run -p 9090:9090 -v ./example/prometheus.yml:/etc/prometheus/prometheus.yml -v prometheus-data:/prometheus prom/prometheus
   ```
### Install Grafana (Testing)
- installation
    ```
    https://grafana.com/docs/grafana/latest/setup-grafana/installation/debian/
    #sudo apt-get install -y apt-transport-https software-properties-common wget
    #sudo mkdir -p /etc/apt/keyrings/
    #wget -q -O - https://apt.grafana.com/gpg.key | gpg --dearmor | sudo tee /etc/apt/keyrings/grafana.gpg > /dev/null
    #echo "deb [signed-by=/etc/apt/keyrings/grafana.gpg] https://apt.grafana.com stable main" | sudo tee -a /etc/apt/sources.list.d/grafana.list
    #sudo apt-get update
    #sudo apt-get install grafana

    ```
- running
    ```
    sudo systemctl daemon-reload
    sudo systemctl start grafana-server
    sudo systemctl status grafana-server
    ```

## Documentation

For detailed documentation including installation guides, configuration options, and metric descriptions, see the [documentation](https://instinct.docs.amd.com/projects/device-metrics-exporter/en/latest).
