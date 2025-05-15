# Overview
`metricutil` can work in two modes:
- pull once mode: pull metric data or parse local file one time
- watch mode: continousely pull metric data from given endpoint, and show the changed metrics or all metrics

# Pull once mode
The tool can parse local file or remote http endpoint, and convert data in json format
- parse local file: `metricutil -o output.json ./metrics.txt`
- parse remote url: `metricutil -o output.json http://<ip:port>/metrics

## Output json format
Below is one sample json. It is an list of metric item.
```
[
    {
        "name": "gpu_nodes_total",
        "help": "Number of nodes with GPUs",
        "type": 1,
        "metric": [
            {
                "gauge": {
                    "value": 1
                }
            }
        ]
    },
    {
        "name": "gpu_power_usage",
        "help": "power usage in Watts",
        "type": 1,
        "metric": [
            {
                "label": [
                    {
                        "name": "GPU_UUID",
                        "value": "6699c0a8-4200-4242-037f-f97e6cae0f09"
                    },
                    {
                        "name": "SERIAL_NUMBER",
                        "value": "692247001282"
                    }
                ],
                "gauge": {
                    "value": 41
                }
            }
        ]
    },
    {
        "name": "gpu_total_memory",
        "help": "total VRAM memory of the GPU (in MB)",
        "type": 1,
        "metric": [
            {
                "label": [
                    {
                        "name": "GPU_UUID",
                        "value": "6699c0a8-4200-4242-037f-f97e6cae0f09"
                    },
                    {
                        "name": "SERIAL_NUMBER",
                        "value": "692247001282"
                    }
                ],
                "gauge": {
                    "value": 68702
                }
            }
        ]
    },
    {
        "name": "gpu_usage",
        "help": "Current usage as percentage of time the GPU is busy.",
        "type": 1,
        "metric": [
            {
                "label": [
                    {
                        "name": "GPU_UUID",
                        "value": "6699c0a8-4200-4242-037f-f97e6cae0f09"
                    },
                    {
                        "name": "SERIAL_NUMBER",
                        "value": "692247001282"
                    }
                ],
                "gauge": {
                    "value": 0
                }
            }
        ]
    }
]
```
`Type` is enumerated value as below

| Type | Name      |
|------|-----------|
| 0    | Counter   |
| 1    | Gauge     |
| 2    | Summary   |
| 3    | Untyped   |
| 4    | Histogram |
| 5    | Gauge Histogram|

## Watch mode
./metricutil -w <endpoint url>

# Arg usage
```
$ ./metricutil -h
Usage of ./metricutil:
  -a    show all metrics
  -i duration
        interval to pull (default 10s)
  -o string
        output filepath (default "output.json")
  -out-curr string
        raw data output filepath for watch (default "output_curr.txt")
  -out-last string
        raw data output filepath for watch (default "output_last.txt")
  -w    watch mode
```