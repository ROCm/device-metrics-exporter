# device-metrics-exporter
Device Metrics Exporter exports metrics from AMD GPUs to collectors like Prometheus.
Health Monitoring for each of the GPU is done by the exporter in a 30s
interval. The GPU will be marked as healthy/unhealthy on the grpc service
hosted on /sockets/amdgpu_device_metrics_exporter_grpc.socket for external clients. In case of
Kubernets environment the respective node will have the labels with each gpu
state marked as unhealthy in such case, no labels will be present when gpu is
in healthy state.

_Kubernets Node labels for health monitor_
```
metricsexporter.amd.com.gpu.0.state=unhealthy
```

_Usage of bin/amd-metrics-exporter_
```
  -agent-grpc-port int
      Agent GRPC port (default 50061)
  -amd-metrics-config string
      AMD metrics exporter config file (default "/etc/metrics/config.json")
```
## Supported Platforms
  - Rhel 9.4, Azure Linux 3.0

## ROCM version
  - ROCM 6.2, 6.3

## Build and Run Instructions

### Build amdexporter application binary
-  Run the following make target in the TOP directory. This will also generate the required protos to build the amdexporter application
   	binary.
   	```
    cd $TOPDIR
    make all
    ```
   	

### Build exporter container
-  Run the following make target in the TOP directory:
   	```
    cd $TOPDIR
    make docker
    ```
### Run exporter
  - docker environment
    - To run the exporter container after building the container in $TOPDIR/docker, run:
      
    ```
    cd $TOPDIR/docker/obj
    tar xzf exporter-release-v1.tgz
    ./start_exporter.sh -d docker/exporter-docker-v1.tgz
    ```
      
    - To run the exporter from docker registery
    ```
    docker run -itd --device=/dev/dri --device=/dev/kfd -p 5000:5000 --name exporter rocm/device-metrics-exporter:v1.2.1
    ```
    ```
    # mount /var/run/slurm/  to receive slurm notifications
    -v /var/run/slurm/:/var/run/slurm/
    ```
   - ubuntu linux debian package
     - Supported ROCM versions : 6.2.0 and up
     - prerequistes
       - dkms installated on the system
      - Services run on following default ports. These can be changed by updating
    the respective service file with the below option
    
        - gpuagent - default port 50061 : changing this port would require amd-metrics-exporter to be configured with the port as these services are dependent
    
        `gpuagent -p <grpc_port>`

        - exporter http port is configurable through the config file **ServerPort** filed in /etc/metrics/config.json : please refer to the example/export_configs.json
          ```
          amd-metrics-exporter - defualt port 5000
               -agent-grpc-port <grpc_port>
           ```
        

  - if running unsupported rocm then the behavior is undefined and some metric fields may not work as intended update the **LD_LIBRARY_PATH** in _/usr/local/etc/metrics/gpuagent.conf_ to
    proper library location after installation and before starting the
    services. 
    -   the following libraries must be installed onto the new Library
    path or the system with below command

        `apt-get install -y libdrm libdrm-amdgpu1`

  -  package installation
    ```
    $ dpkg -i amdgpu-exporter_1.2.1_amd64.deb
    ```

  - default config file path _/etc/metrics/config.json_
 
  - to change to a custom conifg file, update
    _/lib/systemd/system/amd-metrics-exporter.service_

    **ExecStart=/usr/local/bin/amd-metrics-exporter -f <custom_config_path>**


  - enable on system bootup (Optional)
    ```
    systemctl enable amd-metrics-exporter.service
    ```

  - starting services
    ```
    systemctl start amd-metrics-exporter.service
    ```

  - stopping service
    ```
    systemctl stop gpuagent.service
    systemctl stop amd-metrics-exporter.service
    ```

  - uninstall package
    ```
    apt-get remove amdgpu-exporter
    ```

  - slurm lua plugin file for metrics job id integrations, this can be copied
    onto the slurm plugin directory to job labels on metrics.
    path : `/usr/local/etc/metrics/pensando.lua`
    proto : `/usr/local/etc/metrics/plugin.proto`
### Default config behavior
- ServerPort : 5000
- Labels Defaults : `gpu_id, serial_number, card_model, hostname, gpu_partition_id, gpu_compute_partition_type`
- Fields Defaults : all fields supported

### Custom metrics config
- To run the exporter with config mount the /etc/metrics/config.json on the
  exporter container 
    - create your config in directory `config/config.json`
    - start docker container
     ```
     docker run -itd --device=/dev/dri --device=/dev/kfd -v ./config:/etc/metrics -p 5000:5000 --name exporter rocm/device-metrics-exporter:v1.2.1
     ```
- The update to config file will take affect graciously without needing the
  container to be restarted. The new config will take effect in less than 1 minute interval.
### Metrics Config formats
- Json file with the following optional keys are expected
    - ServerPort : <port number>
        - this field is ignored when metrics exporter is deployed through
          gpu-operator as to avoid the service node port config causing issues
    - GPUConfig :
        - Fields
            array of string specifying what field to be exported
            present in [_internal/amdgpu/proto/fields.proto_:**GPUMetricField**](https://github.com/pensando/device-metrics-exporter/blob/main/internal/amdgpu/proto/exporterconfig.proto#L32)
        - Labels
            CARD_MODEL, GPU_ID, HOSTNAME, SERIAL_NUMBER, GPU_PARTITION_ID and GPU_COMPUTE_PARTITION_TYPE are always set and cannot be removed. Labels supported are available in
            [_internal/amdgpu/proto/fields.proto_**:GPUMetricLabel**](https://github.com/pensando/device-metrics-exporter/blob/main/internal/amdgpu/proto/exporterconfig.proto#L114)
        - HealthThresholds:
            These values dictates the threshold of the ECC field counters to mark
            a GPU as unhealthy. Default is 0 if not specified, the GPU will be
            marked unhealthy once the value goes above the set threshold limit.
        - CustomLabels
            map of custom labels and values to be exported. Exporter supports a maximum of 10 custom labels currently. Mandatory labels mentioned above supplied as custom labels will
            be ignored.

- Invalid values in any of the field will be ignored and revert to default
  behavior for the respective fields.

### E2e Testing
- The current testing will exercise only the exporter module part with mocked
  external entities

- All the tests are under `test/e2e`

- Running test from TOP directory. This will build the necessary components
  and docker container image packed with mocked dependent services and run all
  the tests.
  `make e2e`

### E2e Kubenetes Testing
- The test expects kubeconfig to run the actual exporter on the amd gpu server
  to test all functionality

- Running test from TOP direcotry.
  `KUBECONFIG=~/.kube/config  make k8s-e2e

- to set more configuration options for the e2e test, run as per below example
  from TOPDIR/test/k8s-e2e
  ```
  go test -helmchart TOPDIR/helm-charts/ -registry 10.11.18.9:5000/amd/exporter -imagetag test -kubeconfig kubeconfig  -namespace test-exporter -v
  ```

### Slurm integration
There are 2 options to collect job information from slurm
#### Using slurm Prolog/Epilog,
   - copy /usr/local/etc/metrics/slurm/slurm-prolog.sh to /etc/slurm/
   - copy /usr/local/etc/metrics/slurm/slurm-epilog.sh to /etc/slurm/
   - chmod +x /etc/slurm/slurm-*.sh to add executable permissions
   - remove /var/run/exporter/ to cleanup if there are no active jobs
   - configure prolog/epilog in slurm.conf,
 ```
   PrologFlags=Alloc
   Prolog=/etc/slurm/slurm-prolog.sh
   Epilog=/etc/slurm/slurm-epilog.sh
   ```

These slurm labels can be configured to export in config.json
```
    JOB_ID
    JOB_USER
    JOB_PARTITION
    CLUSTER_NAME
   ```
  example metrics
```
  gpu_junction_temperature{card_model="0x1002,cluster_name="genoacluster",driver_version="6.8.0-40-generic",gpu_id="0",gpu_uuid="72ff740f-0000-1000-804c-3b58bf67050e",job_id="130",job_partition="LocalQ",job_user="vm",serial_number="692251001124"} 30
```

#### Metrics exporter using SPANK((Slurm Plug-in Architecture for Node and job (K)control)  plugin to collect job metrics
   - Configure SPANK config, plugstack.conf(default) on  worker nodes
   - Copy metrics exporter plugin files from /etc/metrics/slurm to slurm config (/etc/slurm)
   - Restart slurmd service
   - Include JOB_ID in exported labels (config.json)

   metrics will be reported with slurm JOB_IDs, example
```
gpu_edge_temperature{CARD_MODEL="0xc34",DRIVER_VERSION="6.8.5",GPU_ID="0",GPU_UUID="0beb0a09-4200-4242-0e05-67bf583b4c72",JOB_ID="32",SERIAL_NUMBER="692251001124"} 32
```

### Grafana Dashboards
Set of dashboards are provided for exporting onto grafana which displays GPU metrics collected from device-metrics-exporter via a metric endpoint added to Prometheus.
All dashboard json are provided under directory `grafana`
1. _dashboard_overview.json_ - Gives a high level bird eye view of the cluster of GPUs.
2. _dashboard_gpu.json_ - Gives detailed view of each GPU specific to associated host.
3. _dashboard_job.json_ - Gives job level GPU usage detailed view in SLURM and Kubernetes enrivonments.
4. _dashboard_node.json_ - Gives host level GPU usage detailed view.

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

### Debian Complete Installation : Ubuntu 22.04
  - Installation prerequisites
    - DKMS Installation and Setup
        [DKMS detailed
        installation](https://rocm.docs.amd.com/projects/install-on-linux/en/latest/install/amdgpu-install.html)

		Download Package
		[ubuntu 22.04 jammy]
        ```
        wget https://repo.radeon.com/amdgpu-install/6.3.1/ubuntu/jammy/amdgpu-install_6.3.60301-1_all.deb
        ```
		
		[ubuntu 24.04 noble]
		```
		wget https://repo.radeon.com/amdgpu-install/6.3.1/ubuntu/noble/amdgpu-install_6.3.60301-1_all.deb
		```

		```
 		apt install ./amdgpu-install_6.3.60301-1_all.deb 
 		apt --fix-broken install
        apt install ./amdgpu-install_6.3.60301-1_all.deb
        amdgpu-install --usecase=dkms
		modprobe amdgpu
        ```
    - AMD Metrics Exporter Installation and Setup 
        ```
        dpkg -i amdgpu-exporter_1.2.1_amd64.deb

        systemctl enable amd-metrics-exporter.service
        systemctl start amd-metrics-exporter.service
        ```

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
      repository: registry.test.pensando.io:5000/device-metrics-exporter/exporter
      tag: latest
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
    ```helm install exporter ./device-metrics-exporter-charts-v1.2.1.tgz -n mynamespace -f values.yaml```
- Update config:
  - Option 1: you can directly modify the Kubernetes resource to modify the config, including modifying configmap, service, rbac or daemonset resources.
  - Option 2: you can prepare the updated ```values.yaml``` and do a helm chart upgrade: ```helm upgrade exporter -n mynamespace -f updated_values.yaml```
- Uninstallation
  - Uninstall the helm charts by running: 
    ```helm uninstall exporter -n mynamespace```

### tech support collection
    - run this on master/worker node of k8s
    - run from TOP_DIR `./tools/techsupport_dump.sh` with necessary arguments
    ```
    ./tools/techsupport_dump.sh -r test all
    ```
