/*
 Copyright (c) Advanced Micro Devices, Inc. All rights reserved.

 Licensed under the Apache License, Version 2.0 (the \"License\");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an \"AS IS\" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/ROCm/device-metrics-exporter/internal/amdgpu/gen/luaplugin"
	zmq "github.com/go-zeromq/zmq4"
	"google.golang.org/protobuf/proto"
	"log"
	"strings"
)

func main() {
	var jobId int
	var gpuList string
	var jobEnd bool

	flag.IntVar(&jobId, "jobId", 123, "job id ")
	flag.StringVar(&gpuList, "gpuList", "0", "list of gpus")
	flag.BoolVar(&jobEnd, "jobEnd", false, "job exit")
	flag.Parse()

	sock := zmq.NewPush(context.Background())

	if err := sock.Dial("tcp://127.0.0.1:6601"); err != nil {
		log.Fatalf(fmt.Sprintf("failed to dial %v", err))
	}

	msg := luaplugin.Notification{
		Type: map[bool]luaplugin.Stages{false: luaplugin.Stages_TaskInit, true: luaplugin.Stages_TaskExit}[jobEnd],
		SData: &luaplugin.SpankData{
			JobID:     uint32(jobId),
			AllocGPUs: strings.Split(gpuList, ","),
		},
	}
	fmt.Printf("job notification type:%v %+v", msg.Type, msg.SData)

	data, err := proto.Marshal(&msg)
	if err != nil {
		log.Fatal(err)
	}
	if err := sock.Send(zmq.NewMsg(data)); err != nil {
		log.Fatal(fmt.Sprintf("failed to send %v", err))
	}
}
