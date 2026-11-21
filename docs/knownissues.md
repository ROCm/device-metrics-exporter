# Known issues and limitations

## Limitations

- When the exporter runs in a VM guest, the following labels are empty because of an
  SR-IOV restriction from the hypervisor:
  - `card_model`
  - `serial_number`

See [issue 171](https://github.com/ROCm/device-metrics-exporter/issues/171).
