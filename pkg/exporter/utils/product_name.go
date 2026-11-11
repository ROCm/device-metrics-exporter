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

package utils

import "strings"

// productNameReplacer strips parentheses and replaces "/" with a space so that
// subsequent Fields+Join collapses all separator runs into a single "_".
var productNameReplacer = strings.NewReplacer("(", "", ")", "", "/", " ")

// NormalizeProductName returns a sysfs product_name with "(", ")" dropped
// and runs of whitespace or "/" joined by a single "_".
func NormalizeProductName(name string) string {
	return strings.Join(strings.Fields(productNameReplacer.Replace(name)), "_")
}
