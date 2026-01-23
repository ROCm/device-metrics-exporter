# Mock Testing Tool Usage and Notes

The exporter is packed with `metricsclient` for debugging and testing some of the workflows with mocking support.

```bash
$ metricsclient -h
Usage of metricsclient:
  -ecc-file-path string
        json ecc err file
  -get
        send get req
  -id string
        send gpu id (default "1")
  -json
        output in json format
  -label
        get k8s node label
  -socket string
        metrics grpc socket path (default
        "/sockets/amdgpu_device_metrics_exporter_grpc.socket")

```

## 1. Show GPU Health

```bash
[root@e2e-test-k8s-amdgpu-metrics-exporter-n8lvh ~]# metricsclient
ID      Health  Associated Workload
------------------------------------------------
0       healthy []
------------------------------------------------
```

## 2. Inject Mock ECC Error

To simulate ECC error, create a JSON file of the below format with GPU ID, the fields set to ECC fields and counts to respective fields to be updated, and issue the below command. This will print the previous reported health status of the exporter and set of counters mocked.

```bash
[root@e2e-test-k8s-amdgpu-metrics-exporter-n8lvh ~]# cat ecc.json
{
        "ID": "0",
        "Fields": [
                "GPU_ECC_UNCORRECT_SEM",
                "GPU_ECC_UNCORRECT_FUSE"
        ],
        "Counts" : [
                1, 2
        ]
}
[root@e2e-test-k8s-amdgpu-metrics-exporter-n8lvh ~]#
[root@e2e-test-k8s-amdgpu-metrics-exporter-n8lvh ~]# metricsclient -ecc-file-path ecc.json
ID      Health  Associated Workload
------------------------------------------------
0       healthy []
------------------------------------------------
{"ID":"0","Fields":["GPU_ECC_UNCORRECT_SEM","GPU_ECC_UNCORRECT_FUSE"]}
```

## 3. Remove ECC Mock Error

To remove mock fields, set the respective field count values to 0 in the JSON file.

```bash
[root@e2e-test-k8s-amdgpu-metrics-exporter-n8lvh ~]# cat ecc_delete.json
{
        "ID": "0",
        "Fields": [
                "GPU_ECC_UNCORRECT_SEM",
                "GPU_ECC_UNCORRECT_FUSE"
        ],
        "Counts" : [
                0, 0
        ]
}
[root@e2e-test-k8s-amdgpu-metrics-exporter-n8lvh ~]# metricsclient -ecc-file-path ecc_delete.json
ID      Health  Associated Workload
------------------------------------------------
0       unhealthy       []
------------------------------------------------
{"ID":"0","Fields":["GPU_ECC_UNCORRECT_SEM","GPU_ECC_UNCORRECT_FUSE"]}
```

## 4. Setup Mock In-Band RAS Error List

To setup a mock in-band RAS error list file, use the `-setup-mock-inbandras` flag. This will automatically create a `/mockdata/inband-ras/error_list` file with GPU UUIDs from the system.

```bash
[root@e2e-test-k8s-amdgpu-metrics-exporter-n8lvh ~]# metricsclient -setup-mock-inbandras
Successfully created mock inband RAS error_list at /mockdata/inband-ras/error_list
Added 2 GPU(s) to the mock error list
```

The generated file will have the following JSON format with GPU UUID and AFId (AMD Field ID) fields:

```json
{
  "cper": [
    {
      "gpu": "51ff74a1-0000-1000-802e-2769a3f6a69e",
      "afid": []
    },
    {
      "gpu": "61ff74a1-0000-1000-802e-2769a3f6a69e",
      "afid": []
    }
  ]
}
```

You can manually edit this file to add specific AFId values (array of uint64) to simulate CPER (Common Platform Error Record) entries for testing in-band RAS error handling. The `afid` field accepts an array of AMD Field IDs associated with the GPU errors.

Example with AFId values:

```json
{
  "cper": [
    {
      "gpu": "51ff74a1-0000-1000-802e-2769a3f6a69e",
      "afid": [55]
    },
    {
      "gpu": "61ff74a1-0000-1000-802e-2769a3f6a69e",
      "afid": [65]
    }
  ]
}
```

```bash
curl http://localhost:5000/metrics | grep -i afid
gpu_afid_errors{afid_index="0", ... gpu_uuid="51ff74a1-0000-1000-802e-2769a3f6a69e"} 55
gpu_afid_errors{afid_index="0", ... gpu_uuid="61ff74a1-0000-1000-802e-2769a3f6a69e"} 65
```

## 5. Remove Mock In-Band RAS Error List

To remove the mock in-band RAS, delete the `/mockdata` directory using `rm -rf /mockdata`.
