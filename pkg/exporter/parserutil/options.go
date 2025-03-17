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

package parserutil

import (
	"fmt"
	"strconv"
	"strings"
)

func RangeStrToIntIndices(b string) ([]int, error) {
	var indices []int
	numbers := strings.Split(b, ",")
	for _, numOrRange := range numbers {
		token := strings.Split(numOrRange, "-")
		tokenCount := len(token)
		if tokenCount > 2 {
			return indices, fmt.Errorf("range must be of format 'min-max', but found '%s'", numOrRange)
		} else if tokenCount == 1 {
			number, err := strconv.Atoi(token[0])
			if err != nil {
				return indices, err
			}
			indices = append(indices, number)
		} else {
			start, err := strconv.Atoi(token[0])
			if err != nil {
				return indices, err
			}
			end, err := strconv.Atoi(token[1])
			if err != nil {
				return indices, err
			}
			if start > end {
				return indices, fmt.Errorf("range must be of format 'min-max', but found '%s'", numOrRange)
			}

			// Add the range to the indices
			for i := start; i <= end; i++ {
				indices = append(indices, i)
			}
		}
	}
	return indices, nil
}
