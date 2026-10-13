/*
Copyright (c) Advanced Micro Devices, Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package snapshotstore

import (
	"sort"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// DeviceKey builds the per-device store key from a label set: the non-skipped
// label names are sorted and joined as k=v with 0x1f separators. GPU and NIC
// writers share it so their key derivations cannot drift. skip may be nil.
func DeviceKey(labels prometheus.Labels, skip map[string]struct{}) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		if _, omit := skip[k]; omit {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(0x1f)
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(labels[k])
	}
	return b.String()
}
