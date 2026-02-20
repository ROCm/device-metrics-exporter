# Troubleshooting Device Metrics Exporter

This topic provides an overview of troubleshooting options for Device Metrics Exporter.

## Techsupport Collection

### K8s Techsupport Collection

The [techsupport-dump script](https://github.com/ROCm/device-metrics-exporter/blob/main/tools/techsupport_dump.sh) can be used to collect system state and logs for debugging:

```bash
# ./techsupport_dump.sh [-w] [-o yaml/json] [-k kubeconfig] [-r helm-release-name] <node-name/all>
```

Options:

- `-w`: wide option
- `-o yaml/json`: output format (default: json)
- `-k kubeconfig`: path to kubeconfig (default: ~/.kube/config)
- `-r  helm-release-name`: helm release name

### Docker Techsupport Collection

```bash
docker exec -it device-metrics-exporter metrics-exporter-ts.sh
docker cp device-metrics-exporter:/var/log/amd-metrics-exporter-techsupport-<timestamp>.tar.gz .
```

### Debian Techsupport Collection

```bash
sudo metrics-exporter-ts.sh
```

Please file an issue with collected techsupport bundle on our [GitHub Issues](https://github.com/ROCm/device-metrics-exporter/issues) page

## Logs

You can view the container logs by executing the following command:

### K8s deployment

```bash
kubectl logs -n <namespace> <exporter-container-on-node>
```

### Docker deployment

```bash
docker logs device-metrics-exporter
```

### Debian deployment

```bash
sudo journalctl -xu amd-metrics-exporter
```

## Common Issues

This section describes common issues with AMD Device Metrics Exporter

1. Port conflicts:
   - Verify port 5000 is available
   - Configure an alternate port through the configuration file

2. Device access:
   - Ensure proper permissions on `/dev/dri` and `/dev/kfd`
   - Verify ROCm is properly installed

3. Metric collection issues:
   - Check GPU driver status
   - Verify ROCm version compatibility

4. App Armor blocking Profiler:

```bash
# dmesg  | grep -3 rocpctl
root@genoa3:~/praveen# dmesg | grep -3 rocpctl
[97478.776746] cni0: port 10(veth9ec08a32) entered forwarding state
[113647.022518] audit: type=1400 audit(1765338835.280:130): apparmor="DENIED" operation="open" class="file" profile="ubuntu_pro_apt_news" name="/opt/rocm-7.1.1/lib/" pid=801116 comm="python3" requested_mask="r" denied_mask="r" fsuid=0 ouid=0
[113647.029634] audit: type=1400 audit(1765338835.287:131): apparmor="DENIED" operation="open" class="file" profile="ubuntu_pro_esm_cache" name="/opt/rocm-7.1.1/lib/" pid=801117 comm="python3" requested_mask="r" denied_mask="r" fsuid=0 ouid=0
[172955.500614] rocpctl[1200455]: segfault at 736d ip 00007279ae77f98e sp 00007ffea7539590 error 4 in librocprofiler-sdk.so.1.0.0[7279ae22a000+69e000] likely on CPU 42 (core 82, socket 0)
[172955.500630] Code: 40 31 d2 48 8b 5d 38 4c 8b 00 4c 89 c0 48 f7 f6 4c 8d 2c d3 49 89 d3 4d 8b 55 00 4d 85 d2 0f 84 9d 00 00 00 49 8b 02 4d 89 d1 <48> 8b 48 08 4c 39 c1 74 28 48 8b 38 48 85 ff 0f 84 82 00 00 00 48
[172955.598083] amdgpu: Freeing queue vital buffer 0x727888200000, queue evicted
[172955.598090] amdgpu: Freeing queue vital buffer 0x727890200000, queue evicted
```

  **Solution** : Disable App Armor or create custom profile to allow `rocpctl` access to /opt/rocm-7.1.1/lib/
