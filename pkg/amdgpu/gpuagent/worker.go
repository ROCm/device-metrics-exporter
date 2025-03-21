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

package gpuagent

import (
	"context"
	"runtime"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gen/amdgpu"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/logger"
	"github.com/gofrs/uuid"
	"golang.org/x/sync/errgroup"
)

const (
	maxWorkers  = 5
	maxJobQueue = 16
)

type workerJob struct {
	id  int
	req *amdgpu.GPUGetRequest
}

func NewWokerRequest(ctx context.Context, gpuclient amdgpu.GPUSvcClient, gpuids map[string][]byte) ([]*amdgpu.GPUGetResponse, error) {
	gpus := len(gpuids)
	totalWorkers := maxWorkers
	numCores := runtime.NumCPU()

	if numCores < maxWorkers {
		totalWorkers = numCores
	}
	if gpus < totalWorkers {
		totalWorkers = gpus
	}
	//logger.Log.Printf("total workers[%v] totalWorkers", totalWorkers)
	jobQueue := make(chan *workerJob, maxJobQueue)
	results := make([]*amdgpu.GPUGetResponse, gpus)
	eg, ectx := errgroup.WithContext(ctx)

	work := func(ctx context.Context, job *workerJob) error {
		select {
		case <-ctx.Done():
			uuid, _ := uuid.FromBytes(job.req.Id[0])
			logger.Log.Printf("Canceled the job [%d] for gpuid[%v]", job.id, uuid)
			return nil
		default:
			res, err := gpuclient.GPUGet(ctx, job.req)
			if err != nil {
				return err
			}
			results[job.id] = res
			return nil
		}

	}
	// intialize total workers
	for i := 0; i < totalWorkers; i++ {
		eg.Go(func() error {
			for j := range jobQueue {
				err := work(ectx, j)
				if err != nil {
					return err
				}
			}
			return nil
		})
	}

	// send all the jobs to worker queue
	jobId := 0
	for _, gpuid := range gpuids {
		req := &amdgpu.GPUGetRequest{
			Id: [][]byte{
				gpuid,
			},
		}
		job := &workerJob{
			id:  jobId,
			req: req,
		}
		jobQueue <- job
		jobId = jobId + 1
	}
	close(jobQueue)
	err := eg.Wait()

	return results, err
}
