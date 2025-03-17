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

package metricsserver

import (
	"context"
	"sync"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/testsvc"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"google.golang.org/protobuf/types/known/emptypb"
)

type TestSvcImpl struct {
	sync.Mutex // move to readwrite mutex
	testsvc.UnimplementedTestServiceServer
}

func (t *TestSvcImpl) GetTestResult(ctx context.Context, req *testsvc.TestGetRequest) (*testsvc.TestGetResponse, error) {
	logger.Log.Printf("Got GetTestResult req: %+v", req)
	return &testsvc.TestGetResponse{}, nil
}

func (t *TestSvcImpl) SubmitTestResult(ctx context.Context, req *testsvc.TestPostRequest) (*testsvc.TestGetResponse, error) {
	logger.Log.Printf("Got SubmitTestResult req: %+v", req)
	return &testsvc.TestGetResponse{}, nil
}

func (t *TestSvcImpl) List(ctx context.Context, e *emptypb.Empty) (*testsvc.TestGetResponse, error) {
	logger.Log.Printf("Got List req")
	return &testsvc.TestGetResponse{}, nil
}

func (t *TestSvcImpl) mustEmbedUnimplementedTestServiceServer() {}

func newTestServer() *TestSvcImpl {
	t := &TestSvcImpl{}
	return t
}
