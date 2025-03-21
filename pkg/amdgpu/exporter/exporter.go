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

package exporter

import (
	"context"
	"expvar"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"path"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/config"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gpuagent"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/metricsutil"
	metricsserver "github.com/ROCm/device-metrics-exporter/pkg/amdgpu/svc"
	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/utils"
)

const (
	metricsHandlerPrefix = "/metrics"
)

var (
	mh        *metricsutil.MetricsHandler
	gpuclient *gpuagent.GPUAgentClient
	runConf   *config.ConfigHandler
)

// ExporterOption set desired option
type ExporterOption func(e *Exporter)

// Exporter Handler
type Exporter struct {
	agentGrpcPort int
	configFile    string
	zmqDisable    bool
}

// get the info from gpu agent and update the current metrics registery
func prometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		url := r.URL.String()
		if strings.Contains(strings.ToLower(url), metricsHandlerPrefix) {
			// pull metrics only for metrics handler
			_ = mh.UpdateMetrics()
		}
		next.ServeHTTP(w, r)
	})
}

func startMetricsServer(c *config.ConfigHandler) *http.Server {

	serverPort := c.GetServerPort()

	router := mux.NewRouter()
	router.Use(prometheusMiddleware)

	reg := mh.GetRegistry()
	router.Handle(metricsHandlerPrefix, promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	// pprof
	router.Methods("GET").Subrouter().Handle("/debug/vars", expvar.Handler())
	router.Methods("GET").Subrouter().HandleFunc("/debug/pprof/", pprof.Index)
	router.Methods("GET").Subrouter().HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	router.Methods("GET").Subrouter().HandleFunc("/debug/pprof/profile", pprof.Profile)
	router.Methods("GET").Subrouter().HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	router.Methods("GET").Subrouter().HandleFunc("/debug/pprof/trace", pprof.Trace)
	router.Methods("GET").Subrouter().HandleFunc("/debug/pprof/allocs", pprof.Handler("allocs").ServeHTTP)
	router.Methods("GET").Subrouter().HandleFunc("/debug/pprof/block", pprof.Handler("block").ServeHTTP)
	router.Methods("GET").Subrouter().HandleFunc("/debug/pprof/heap", pprof.Handler("heap").ServeHTTP)
	router.Methods("GET").Subrouter().HandleFunc("/debug/pprof/mutex", pprof.Handler("mutex").ServeHTTP)
	router.Methods("GET").Subrouter().HandleFunc("/debug/pprof/goroutine", pprof.Handler("goroutine").ServeHTTP)
	router.Methods("GET").Subrouter().HandleFunc("/debug/pprof/threadcreate", pprof.Handler("threadcreate").ServeHTTP)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%v", serverPort),
		Handler: router,
	}

	go func() {
		logger.Log.Printf("serving requests on port %v", serverPort)
		err := srv.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe(): %v", err)
		}
		logger.Log.Printf("server on port %v shutdown gracefully", serverPort)
	}()
	return srv
}

func foreverWatcher() {
	var srvHandler *http.Server
	configPath := runConf.GetMetricsConfigPath()
	directory := path.Dir(configPath)
	os.MkdirAll(directory, 0755)
	logger.Log.Printf("config directory for watch : %v", directory)

	serverRunning := func() bool {
		return srvHandler != nil
	}

	startServer := func() {
		if !serverRunning() {
			mh.InitConfig()
			serverPort := runConf.GetServerPort()
			logger.Log.Printf("starting server on %v", serverPort)
			srvHandler = startMetricsServer(runConf)

		}
	}
	stopServer := func() {
		if serverRunning() {
			logger.Log.Printf("stopping server")
			srvCtx, srvCancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := srvHandler.Shutdown(srvCtx); err != nil {
				panic(err) // failure/timeout shutting down the server gracefully
			}
			srvCancel()
			time.Sleep(1 * time.Second)
			srvHandler = nil
		}
	}

	// start server and listen for changes later
	startServer()

	// Create new watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Log.Fatal(err)
	}
	defer watcher.Close()

	ctx := context.Background()
	// Start listening for events.
	go func() {
		for ctx.Err() == nil {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// k8s has to many cases to handle because of symlink, to be
				// safe handle all cases
				if event.Has(fsnotify.Create | fsnotify.Write | fsnotify.Remove | fsnotify.Rename) {
					logger.Log.Printf("loading new config on %v", configPath)
					// stop server if running
					stopServer()
					// start server
					startServer()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					logger.Log.Printf("error: %v", err)
					return
				}
			}
		}
	}()

	// Add a path.
	err = watcher.Add(directory)
	if err != nil {
		log.Fatal(err)
	}

	logger.Log.Printf("starting file watcher for %v", configPath)

	<-make(chan struct{})
}

func NewExporter(agentGrpcport int, configFile string, opts ...ExporterOption) *Exporter {
	exporter := &Exporter{
		agentGrpcPort: agentGrpcport,
		configFile:    configFile,
	}
	for _, o := range opts {
		o(exporter)
	}

	return exporter
}

func ExporterWithZmqDisable(zmqDisable bool) ExporterOption {
	return func(e *Exporter) {
		logger.Log.Printf("zmq server disabled")
		e.zmqDisable = zmqDisable
	}
}

// StartMain - doesn't return it exits only on failure
func (e *Exporter) StartMain(enableDebugAPI bool) {

	logger.Init(utils.IsKubernetes())

	svcHandler := metricsserver.InitSvcs(enableDebugAPI)
	go func() {
		logger.Log.Printf("metrics service starting")
		svcHandler.Run()
		logger.Log.Printf("metrics service stopped")
		os.Exit(0)
	}()

	runConf = config.NewConfigHandler(e.configFile, e.agentGrpcPort)

	mh, _ = metricsutil.NewMetrics(runConf)
	mh.InitConfig()

	gpuclient = gpuagent.NewAgent(mh, !e.zmqDisable)
	if err := gpuclient.Init(); err != nil {
		logger.Log.Printf("gpuclient init err :%+v", err)
	}
	defer gpuclient.Close()

	go gpuclient.StartMonitor()

	svcHandler.RegisterHealthClient(gpuclient)

	foreverWatcher()
}

// SetComputeNodeHealth sets the compute node health
func (e *Exporter) SetComputeNodeHealth(health bool) {
	for gpuclient == nil {
		logger.Log.Printf("gpuclient nil, waiting for it to be created")
		time.Sleep(time.Second)
	}
	if gpuclient != nil {
		gpuclient.SetComputeNodeHealthState(health)
	}
}
