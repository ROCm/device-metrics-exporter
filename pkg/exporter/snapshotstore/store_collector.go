/**
# Copyright (c) Advanced Micro Devices, Inc. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the \"License\");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an \"AS IS\" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package snapshotstore

import "github.com/prometheus/client_golang/prometheus"

// FieldDesc maps one store field to a Prometheus metric descriptor. LabelNames
// is the ordered label set; a sample's value for each name is looked up by key.
type FieldDesc struct {
	Field      string
	Name       string
	Help       string
	ValueType  prometheus.ValueType
	LabelNames []string
}

// boundDesc is a FieldDesc with its *prometheus.Desc built once at construction.
type boundDesc struct {
	desc       *prometheus.Desc
	valueType  prometheus.ValueType
	labelNames []string
}

// StoreCollector is a generic prometheus.Collector that renders samples from a
// Store. It emits one const metric per (device snapshot, field) pair for which
// a FieldDesc exists; fields without a descriptor are omitted. Reusable by GPU,
// NIC, and IFOE.
type StoreCollector struct {
	store StoreReader
	descs map[string]boundDesc
}

func NewStoreCollector(store StoreReader, descs []FieldDesc) *StoreCollector {
	m := make(map[string]boundDesc, len(descs))
	for _, d := range descs {
		m[d.Field] = boundDesc{
			desc:       prometheus.NewDesc(d.Name, d.Help, d.LabelNames, nil),
			valueType:  d.ValueType,
			labelNames: d.LabelNames,
		}
	}
	return &StoreCollector{store: store, descs: m}
}

func (c *StoreCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, b := range c.descs {
		ch <- b.desc
	}
}

func (c *StoreCollector) Collect(ch chan<- prometheus.Metric) {
	for _, snap := range c.store.Snapshots() {
		snap.Range(func(field string, s Sample) bool {
			b, ok := c.descs[field]
			if !ok {
				return true
			}
			vals := make([]string, len(b.labelNames))
			for i, ln := range b.labelNames {
				// missing key -> empty string (zero value of map lookup).
				vals[i] = s.Labels[ln]
			}
			ch <- prometheus.MustNewConstMetric(b.desc, b.valueType, s.Value, vals...)
			return true
		})
	}
}
