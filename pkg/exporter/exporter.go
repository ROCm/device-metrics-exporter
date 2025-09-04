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

	"github.com/ROCm/device-metrics-exporter/pkg/amdgpu/gpuagent"
	"github.com/ROCm/device-metrics-exporter/pkg/amdnic/nicagent"
	k8sclient "github.com/ROCm/device-metrics-exporter/pkg/client"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/config"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/metricssvc"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/metricsutil"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
	metricsserver "github.com/ROCm/device-metrics-exporter/pkg/exporter/svc"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
)

var (
	mh                 *metricsutil.MetricsHandler
	gpuclient          *gpuagent.GPUAgentClient
	nicAgent           *nicagent.NICAgentClient
	runConf            *config.ConfigHandler
	debounceDuration   = 3 * time.Second // debounce duration for file watcher
	defaultBindAddress = "0.0.0.0"
)

// ExporterOption set desired option
type ExporterOption func(e *Exporter)

// Exporter Handler
type Exporter struct {
	agentGrpcPort       int
	configFile          string
	zmqDisable          bool
	enableNICMonitoring bool
	enableGPUMonitoring bool
	disableK8sApi       bool
	enableSriov         bool
	bindAddr            string
	k8sApiClient        *k8sclient.K8sClient
	svcHandler          *metricsserver.SvcHandler
	k8sScl              scheduler.SchedulerClient
	ctx                 context.Context
	cancel              context.CancelFunc
}

// get the info from gpu agent and update the current metrics registery
func prometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		url := r.URL.String()
		if strings.Contains(strings.ToLower(url), globals.MetricsHandlerPrefix) {
			// pull metrics only for metrics handler
			_ = mh.UpdateMetrics()
		}
		next.ServeHTTP(w, r)
	})
}

func startMetricsServer(c *config.ConfigHandler, bindAddr string) *http.Server {

	serverPort := c.GetServerPort()

	router := mux.NewRouter()
	router.Use(prometheusMiddleware)

	reg := mh.GetRegistry()
	router.Handle(globals.MetricsHandlerPrefix, promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	// below route is for daemons like node-problem-detector that need all the metrics
	router.Methods("GET").Subrouter().HandleFunc(globals.AMDGPUHandlerPrefix, mh.HandleGPUMetricsQuery)
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

	// enforce some timeouts
	srv := &http.Server{
		Addr:        fmt.Sprintf("%s:%v", bindAddr, serverPort),
		ReadTimeout: 45 * time.Second,
		IdleTimeout: 60 * time.Second,
		Handler:     router,
	}

	go func() {
		logger.Log.Printf("serving requests on %s:%v", bindAddr, serverPort)
		err := srv.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe(): %v", err)
		}
		logger.Log.Printf("server on %s:%v shutdown gracefully", bindAddr, serverPort)
	}()
	return srv
}

func foreverWatcher(e *Exporter) {
	var srvHandler *http.Server
	configPath := runConf.GetMetricsConfigPath()
	directory := path.Dir(configPath)
	if err := os.MkdirAll(directory, 0755); err != nil {
		logger.Log.Printf("Error opening metrics config path: %v", err)
	}
	logger.Log.Printf("config directory for watch : %v", directory)

	serverRunning := func() bool {
		return srvHandler != nil
	}

	startServer := func() {
		if !serverRunning() {
			mh.InitConfig()
			serverPort := runConf.GetServerPort()
			logger.Log.Printf("starting server on %s:%v", e.bindAddr, serverPort)
			srvHandler = startMetricsServer(runConf, e.bindAddr)
			go func() {
				err := e.svcHandler.Run()
				if err != nil {
					logger.Log.Printf("health service start failed")
				}
			}()

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
			e.svcHandler.Stop()
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

	// Start listening for events.
	go func() {
		debounce := time.NewTimer(0)
		if !debounce.Stop() {
			<-debounce.C
		}
		debounce.Reset(debounceDuration)

		for e.ctx.Err() == nil {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Create | fsnotify.Write | fsnotify.Remove | fsnotify.Rename) {
					if !debounce.Stop() {
						select {
						case <-debounce.C:
						default:
						}
					}
					debounce.Reset(debounceDuration)
				}
			case <-debounce.C:
				logger.Log.Printf("loading new config on %v", configPath)
				stopServer()
				startServer()
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
		logger.Log.Fatal(err)
	}

	logger.Log.Printf("starting file watcher for %v", configPath)

	<-e.ctx.Done()
	stopServer()
	logger.Log.Printf("file watcher stopped due to context cancellation")
}

func NewExporter(agentGrpcport int, configFile string, opts ...ExporterOption) *Exporter {
	ctx, cancel := context.WithCancel(context.Background())
	logger.Log.Printf("creating exporter with grpc port %d and config file %s", agentGrpcport, configFile)
	exporter := &Exporter{
		agentGrpcPort: agentGrpcport,
		configFile:    configFile,
		bindAddr:      defaultBindAddress,
		ctx:           ctx,
		cancel:        cancel,
		k8sApiClient:  nil,
		disableK8sApi: false, // by default k8s api is enabled
	}

	for _, o := range opts {
		o(exporter)
	}
	if utils.IsKubernetes() && !exporter.disableK8sApi {
		hostname, _ := utils.GetHostName()
		k8sApiClient, err := k8sclient.NewClient(ctx, "", hostname)
		if err != nil {
			logger.Log.Fatalf("failed to create k8s client: %v", err)
			// if k8s client creation fails, we return nil to indicate that exporter is not ready
			// this will prevent the exporter from starting and allow the caller to handle the error
			// gracefully, e.g., by retrying or logging the error.
			// This is important because the exporter relies on the k8s client for various operations,
			return nil
		} else {
			exporter.k8sApiClient = k8sApiClient
			logger.Log.Printf("k8s client created successfully")
		}
	}

	return exporter
}

func ExporterWithZmqDisable(zmqDisable bool) ExporterOption {
	return func(e *Exporter) {
		logger.Log.Printf("zmq server disabled")
		e.zmqDisable = zmqDisable
	}
}

func WithBindAddr(bindAddr string) ExporterOption {
	return func(e *Exporter) {
		logger.Log.Printf("bind address set to %s", bindAddr)
		e.bindAddr = bindAddr
	}
}

func (e *Exporter) GetK8sApiClient() *k8sclient.K8sClient {
	if utils.IsKubernetes() && !e.disableK8sApi {
		return e.k8sApiClient
	}
	return nil
}

func (e *Exporter) startWatchers() {
	if e.k8sApiClient == nil {
		logger.Log.Printf("k8s client is not initialized, skipping watchers")
		return
	}

	if err := e.k8sApiClient.Watch(); err != nil {
		logger.Log.Printf("failed to start k8s watchers: %v", err)
	} else {
		logger.Log.Printf("k8s watchers started successfully")
	}
}

func WithNICMonitoring(enableNICAgent bool) ExporterOption {
	return func(e *Exporter) {
		logger.Log.Printf("NIC monitoring enable %v", enableNICAgent)
		e.enableNICMonitoring = enableNICAgent
	}
}

func WithGPUMonitoring(enableGPUAgent bool) ExporterOption {
	return func(e *Exporter) {
		logger.Log.Printf("GPU monitoring enable %v", enableGPUAgent)
		e.enableGPUMonitoring = enableGPUAgent
	}
}

func WithSRIOV(enableSriov bool) ExporterOption {
	return func(e *Exporter) {
		logger.Log.Printf("Host SRIOV mode set to %v", enableSriov)
		e.enableSriov = enableSriov
	}
}

func WithNoK8sApiclient() ExporterOption {
	return func(e *Exporter) {
		e.disableK8sApi = true
		e.k8sApiClient = nil
	}
}

// StartMain - doesn't return it exits only on failure
func (e *Exporter) StartMain(enableDebugAPI bool) {
	defer e.Close()
	logger.Init(utils.IsKubernetes())

	runConf = config.NewConfigHandler(e.configFile, e.agentGrpcPort)

	mh, _ = metricsutil.NewMetrics(runConf)
	mh.InitConfig()

	e.svcHandler = metricsserver.InitSvcs(mh,
		metricsserver.WithDebugAPIOption(enableDebugAPI),
		metricsserver.WithNICMonitoring(e.enableNICMonitoring),
		metricsserver.WithGPUMonitoring(e.enableGPUMonitoring),
	)

	// create scheduler client
	if utils.IsKubernetes() && !e.disableK8sApi {
		k8sScl, err := scheduler.NewKubernetesClient(e.ctx)
		if err != nil {
			logger.Log.Printf("failed to create k8s scheduler client: %v", err)
		} else {
			e.k8sScl = k8sScl
		}
		e.startWatchers()
	}

	if e.enableGPUMonitoring {
		gpuclient = gpuagent.NewAgent(mh,
			gpuagent.WithZmq(!e.zmqDisable),
			gpuagent.WithK8sClient(e.GetK8sApiClient()),
			gpuagent.WithSRIOV(e.enableSriov),
			gpuagent.WithK8sSchedulerClient(e.k8sScl),
		)
		if err := gpuclient.Init(); err != nil {
			logger.Log.Printf("gpuclient init err :%+v", err)
		}
		go gpuclient.StartMonitor()
		if err := e.svcHandler.RegisterGPUHealthClient(gpuclient); err != nil {
			logger.Log.Printf("health client registration err: %+v", err)
		}
	}

	if e.enableNICMonitoring {
		nicAgent = nicagent.NewAgent(mh,
			nicagent.WithK8sSchedulerClient(e.k8sScl),
			nicagent.WithK8sClient(e.GetK8sApiClient()),
		)
		if err := nicAgent.Init(); err != nil {
			logger.Log.Printf("nic client init err :%+v", err)
		}
		if err := e.svcHandler.RegisterNICHealthClient(nicAgent); err != nil {
			logger.Log.Printf("nic health client registration err: %+v", err)
		}
		// discover health states during startup
		if _, err := nicAgent.GetNICHealthStates(); err != nil {
			logger.Log.Printf("failed to get NIC health states: %v", err)
		}
	}

	if utils.IsKubernetes() {
		copyFilesToHost()
	}
	// start file watcher for config changes
	foreverWatcher(e)
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

// GetGPUWorkloads get workloads associated with GPU
func (e *Exporter) GetGPUWorkloads() (map[string][]string, error) {
	workloads := map[string][]string{}
	if gpuclient == nil {
		return nil, fmt.Errorf("gpuclient is not ready")
	}

	hstates, err := gpuclient.GetGPUHealthStates()
	if err != nil {
		return nil, fmt.Errorf("health status failed, %v", err)
	}
	for k, v := range hstates {
		if state, ok := v.(*metricssvc.GPUState); ok {
			workloads[k] = append(workloads[k], state.AssociatedWorkload...)
		}
	}
	return workloads, nil
}

// Close - closes the exporter and all its resources
func (e *Exporter) Close() error {
	e.cancel()
	if gpuclient != nil {
		gpuclient.Close()
		gpuclient = nil
	}

	if nicAgent != nil {
		nicAgent.Close()
		nicAgent = nil
	}

	if e.k8sApiClient != nil {
		e.k8sApiClient.Stop()
		e.k8sApiClient = nil
	}
	return nil
}
