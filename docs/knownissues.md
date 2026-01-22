# Known Issues/Limitations

## Limitations

- Exporter In VM guest mode the follow labels will be empty. This is a
  limitation because of SRIOV restriction from hypervisor
  - `card_model`
  - `serial_number`

[Issue 171](https://github.com/ROCm/device-metrics-exporter/issues/171)
