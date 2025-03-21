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

package logger

import (
	"github.com/ROCm/device-metrics-exporter/internal/k8s"
	"log"
	"os"
	"sync"
)

var (
	Log     *log.Logger
	logdir  = "/var/run/"
	logpath = "exporter.log"
	once    sync.Once
)

func initLogger() {
	if k8s.IsKubernetes() {
		Log = log.New(os.Stdout, "exporter ", log.Lmsgprefix)
	} else {
		if os.Getenv("LOGDIR") != "" {
			logdir = os.Getenv("LOGDIR")
		}
		outfile, _ := os.Create(logdir + logpath)
		Log = log.New(outfile, "", 0)
	}

	Log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func Init() {
	once.Do(initLogger)
}
