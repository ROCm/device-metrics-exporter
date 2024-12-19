# Troubleshooting Device Metrics Exporter

This topic provides an overview of troubleshooting options for Device Metrics Exporter.

## Logs

You can view the container logs by executing the following command:

```bash
docker logs device-metrics-exporter
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
