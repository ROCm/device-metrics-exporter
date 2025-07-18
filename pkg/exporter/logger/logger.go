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
	"log"
	"os"
	"path/filepath"
	"sync"
)

var (
	Log       *log.Logger
	logdir    = "/var/log/"
	logfile   = "exporter.log"
	logPrefix = "exporter "
	once      sync.Once
)

// SetLogPrefix sets prefix in the log to be exporter or testrunner
func SetLogPrefix(prefix string) {
	logPrefix = prefix
}

// SetLogFile sets the log file name
func SetLogFile(file string) {
	logfile = file
}

// SetLogDir sets the path to the directory of logs
func SetLogDir(dir string) {
	logdir = dir
}

func initLogger(console bool) {
	if console {
		Log = log.New(os.Stdout, logPrefix, log.Lmsgprefix)
	} else {
		if os.Getenv("LOGDIR") != "" {
			logdir = os.Getenv("LOGDIR")
		}
		outfile, _ := os.Create(filepath.Join(logdir, logfile))
		Log = log.New(outfile, "", 0)
	}

	Log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func Init(console bool) {
	init := func() {
		initLogger(console)
	}
	once.Do(init)
}
