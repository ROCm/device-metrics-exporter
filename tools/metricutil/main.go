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
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

type clockCommon struct {
	Clk struct {
		Value int    `json:"value"`
		Unit  string `json:"unit"`
	} `json:"clk"`
	MinClk struct {
		Value int    `json:"value"`
		Unit  string `json:"unit"`
	} `json:"min_clk"`
	MaxClk struct {
		Value int    `json:"value"`
		Unit  string `json:"unit"`
	} `json:"max_clk"`
	ClkLocked string `json:"clk_locked"`
	DeepSleep string `json:"deep_sleep"`
}

type clockVideoData struct {
	Clk struct {
		Value int    `json:"value"`
		Unit  string `json:"unit"`
	} `json:"clk"`
	MinClk    string `json:"min_clk"`
	MaxClk    string `json:"max_clk"`
	ClkLocked string `json:"clk_locked"`
	DeepSleep string `json:"deep_sleep"`
}

type amdSMIMetrics struct {
	Gpu   int `json:"gpu"`
	Usage struct {
		GfxActivity struct {
			Value int    `json:"value"`
			Unit  string `json:"unit"`
		} `json:"gfx_activity"`
		UmcActivity struct {
			Value int    `json:"value"`
			Unit  string `json:"unit"`
		} `json:"umc_activity"`
		/* N/A at MI300
		MmActivity struct {
			Value int    `json:"value"`
			Unit  string `json:"unit"`
		} `json:"mm_activity"`
		VcnActivity  []string `json:"vcn_activity"`
		JpegActivity []string `json:"jpeg_activity"`
		*/
	} `json:"usage"`
	Power struct {
		SocketPower struct {
			Value int    `json:"value"`
			Unit  string `json:"unit"`
		} `json:"socket_power"`
		GfxVoltage struct {
			Value string `json:"value"`
			Unit  string `json:"unit"`
		} `json:"gfx_voltage"`
		SocVoltage struct {
			Value string `json:"value"`
			Unit  string `json:"unit"`
		} `json:"soc_voltage"`
		MemVoltage struct {
			Value string `json:"value"`
			Unit  string `json:"unit"`
		} `json:"mem_voltage"`
		ThrottleStatus  string `json:"throttle_status"`
		PowerManagement string `json:"power_management"`
	} `json:"power"`
	Clock struct {
		Gfx0  clockCommon `json:"gfx_0"`
		Gfx1  clockCommon `json:"gfx_1"`
		Gfx2  clockCommon `json:"gfx_2"`
		Gfx3  clockCommon `json:"gfx_3"`
		Gfx4  clockCommon `json:"gfx_4"`
		Gfx5  clockCommon `json:"gfx_5"`
		Gfx6  clockCommon `json:"gfx_6"`
		Gfx7  clockCommon `json:"gfx_7"`
		Mem0  clockCommon `json:"mem_0"`
		Vclk0 clockCommon `json:"vclk_0"`
		Vclk1 clockCommon `json:"vclk_1"`
		Vclk2 clockCommon `json:"vclk_2"`
		Vclk3 clockCommon `json:"vclk_3"`
		Dclk0 clockCommon `json:"dclk_0"`
		Dclk1 clockCommon `json:"dclk_1"`
		Dclk2 clockCommon `json:"dclk_2"`
		Dclk3 clockCommon `json:"dclk_3"`
	} `json:"clock"`
	Temperature struct {
		/*
			Edge struct {
				Value int    `json:"value"`
				Unit  string `json:"unit"`
			} `json:"edge"`
		*/
		Hotspot struct {
			Value int    `json:"value"`
			Unit  string `json:"unit"`
		} `json:"hotspot"`
		Mem struct {
			Value int    `json:"value"`
			Unit  string `json:"unit"`
		} `json:"mem"`
	} `json:"temperature"`
	Pcie struct {
		Width int `json:"width"`
		Speed struct {
			Value int    `json:"value"`
			Unit  string `json:"unit"`
		} `json:"speed"`
		Bandwidth struct {
			Value int    `json:"value"`
			Unit  string `json:"unit"`
		} `json:"bandwidth"`
		ReplayCount         int `json:"replay_count"`
		L0ToRecoveryCount   int `json:"l0_to_recovery_count"`
		ReplayRollOverCount int `json:"replay_roll_over_count"`
		NakSentCount        int `json:"nak_sent_count"`
		NakReceivedCount    int `json:"nak_received_count"`
		/*
			CurrentBandwidthSent struct {
				Value int    `json:"value"`
				Unit  string `json:"unit"`
			} `json:"current_bandwidth_sent"`
			CurrentBandwidthReceived struct {
				Value int    `json:"value"`
				Unit  string `json:"unit"`
			} `json:"current_bandwidth_received"`
			MaxPacketSize struct {
				Value int    `json:"value"`
				Unit  string `json:"unit"`
			} `json:"max_packet_size"`
		*/
	} `json:"pcie"`
	Ecc struct {
		TotalCorrectableCount   int `json:"total_correctable_count"`
		TotalUncorrectableCount int `json:"total_uncorrectable_count"`
		TotalDeferredCount      int `json:"total_deferred_count"`
		CacheCorrectableCount   int `json:"cache_correctable_count"`
		CacheUncorrectableCount int `json:"cache_uncorrectable_count"`
	} `json:"ecc"`
	EccBlocks struct {
		Umc struct {
			CorrectableCount   int `json:"correctable_count"`
			UncorrectableCount int `json:"uncorrectable_count"`
			DeferredCount      int `json:"deferred_count"`
		} `json:"UMC"`
		Sdma struct {
			CorrectableCount   int `json:"correctable_count"`
			UncorrectableCount int `json:"uncorrectable_count"`
			DeferredCount      int `json:"deferred_count"`
		} `json:"SDMA"`
		Gfx struct {
			CorrectableCount   int `json:"correctable_count"`
			UncorrectableCount int `json:"uncorrectable_count"`
			DeferredCount      int `json:"deferred_count"`
		} `json:"GFX"`
		Mmhub struct {
			CorrectableCount   int `json:"correctable_count"`
			UncorrectableCount int `json:"uncorrectable_count"`
			DeferredCount      int `json:"deferred_count"`
		} `json:"MMHUB"`
		PcieBif struct {
			CorrectableCount   int `json:"correctable_count"`
			UncorrectableCount int `json:"uncorrectable_count"`
			DeferredCount      int `json:"deferred_count"`
		} `json:"PCIE_BIF"`
		Hdp struct {
			CorrectableCount   int `json:"correctable_count"`
			UncorrectableCount int `json:"uncorrectable_count"`
			DeferredCount      int `json:"deferred_count"`
		} `json:"HDP"`
	} `json:"ecc_blocks"`
	Fan struct {
		Speed string `json:"speed"`
		Max   string `json:"max"`
		Rpm   string `json:"rpm"`
		Usage string `json:"usage"`
	} `json:"fan"`
	VoltageCurve struct {
		Point0Frequency string `json:"point_0_frequency"`
		Point0Voltage   string `json:"point_0_voltage"`
		Point1Frequency string `json:"point_1_frequency"`
		Point1Voltage   string `json:"point_1_voltage"`
		Point2Frequency string `json:"point_2_frequency"`
		Point2Voltage   string `json:"point_2_voltage"`
	} `json:"voltage_curve"`
	Overdrive string `json:"overdrive"`
	PerfLevel string `json:"perf_level"`
	XgmiErr   string `json:"xgmi_err"`
	Energy    struct {
		TotalEnergyConsumption struct {
			Value float64 `json:"value"`
			Unit  string  `json:"unit"`
		} `json:"total_energy_consumption"`
	} `json:"energy"`
	MemUsage struct {
		TotalVram struct {
			Value int    `json:"value"`
			Unit  string `json:"unit"`
		} `json:"total_vram"`
		UsedVram struct {
			Value int    `json:"value"`
			Unit  string `json:"unit"`
		} `json:"used_vram"`
		FreeVram struct {
			Value int    `json:"value"`
			Unit  string `json:"unit"`
		} `json:"free_vram"`
		TotalVisibleVram struct {
			Value int    `json:"value"`
			Unit  string `json:"unit"`
		} `json:"total_visible_vram"`
		UsedVisibleVram struct {
			Value int    `json:"value"`
			Unit  string `json:"unit"`
		} `json:"used_visible_vram"`
		FreeVisibleVram struct {
			Value int    `json:"value"`
			Unit  string `json:"unit"`
		} `json:"free_visible_vram"`
		TotalGtt struct {
			Value int    `json:"value"`
			Unit  string `json:"unit"`
		} `json:"total_gtt"`
		UsedGtt struct {
			Value int    `json:"value"`
			Unit  string `json:"unit"`
		} `json:"used_gtt"`
		FreeGtt struct {
			Value int    `json:"value"`
			Unit  string `json:"unit"`
		} `json:"free_gtt"`
	} `json:"mem_usage"`
}

// return number of gpus, and start index for clock system, video, data
func scanMetrics(exporter map[string]*dto.MetricFamily) (int, map[string]int, map[string]int, map[string]int, error) {
	ids := map[string]struct{}{}
	for _, v := range exporter {
		for _, m := range v.Metric {
			for _, l := range m.GetLabel() {
				if l.GetName() == "GPU_ID" {
					ids[l.GetValue()] = struct{}{}
				}
			}

		}
	}

	// verify if ID is incremental from 0 to len(gpu_id) by 1
	idExist := map[int]struct{}{}
	for i := 0; i < len(ids); i++ {
		idExist[i] = struct{}{}
	}

	for idStr := range ids {
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return 0, nil, nil, nil, err
		}
		delete(idExist, id)
	}
	if len(idExist) > 0 {
		return 0, nil, nil, nil, fmt.Errorf("GPU_ID %v should exist, but not found", idExist)
	}

	// init index map
	sysIndexes := make(map[string]int)
	videoIndexes := make(map[string]int)
	dataIndexes := make(map[string]int)
	for id := range ids {
		sysIndexes[id] = 1 << 31
		videoIndexes[id] = 1 << 31
		dataIndexes[id] = 1 << 31
	}

	var (
		err        error
		clockType  string
		clockIndex int
	)

	for k, v := range exporter {
		switch k {
		case "gpu_clock":
			for _, m := range v.Metric {
				gpuID := ""
				for _, l := range m.GetLabel() {
					switch l.GetName() {
					case "GPU_ID":
						gpuID = l.GetValue()
					case "clock_type":
						clockType = l.GetValue()
					case "clock_index":
						clockIndex, err = strconv.Atoi(l.GetValue())
						if err != nil {
							return 0, nil, nil, nil, err
						}
					}
				}
				switch clockType {
				case "GPU_CLOCK_TYPE_SYSTEM":
					sys := sysIndexes[gpuID]
					if clockIndex < sys {
						sysIndexes[gpuID] = clockIndex
					}
				case "GPU_CLOCK_TYPE_DATA":
					data := dataIndexes[gpuID]
					if clockIndex < data {
						dataIndexes[gpuID] = clockIndex
					}
				case "GPU_CLOCK_TYPE_VIDEO":
					video := videoIndexes[gpuID]
					if clockIndex < video {
						videoIndexes[gpuID] = clockIndex
					}
				}
			}
		}
	}

	return len(ids), sysIndexes, videoIndexes, dataIndexes, nil
}

func parseMF(reader io.Reader) (map[string]*dto.MetricFamily, error) {
	var parser expfmt.TextParser
	mf, err := parser.TextToMetricFamilies(reader)
	if err != nil {
		return nil, err
	}
	return mf, nil
}

func main() {
	o := flag.String("o", "output.json", "output filepath")
	outCurr := flag.String("out-curr", "output_curr.txt", "current iteration raw data filepath for watch")
	outLast := flag.String("out-last", "output_last.txt", "last iteration raw data filepath for watch")
	w := flag.Bool("w", false, "watch mode")
	outAmdSMI := flag.String("out-amd-smi", "output_amd_smi.json", "amd-smi raw data filepath for watch")
	interval := flag.Duration("i", 5*time.Second, "interval to pull")

	flag.Parse()

	if flag.NArg() == 0 {
		fmt.Println("Please supply one input source")
		return
	}

	if *w {
		watchMetric(flag.Args()[0], *outCurr, *outLast, *outAmdSMI, *interval)
		return
	}

	err := process(flag.Args()[0], *o)
	if err != nil {
		log.Fatalln(err)
	}

	fmt.Printf("save result into %s\n", *outCurr)
}

func clearTerminal() {
	cmd := exec.Command("clear") // Use "cls" for Windows
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func getValue(m *dto.MetricFamily) string {
	switch *m.Type {
	case dto.MetricType_COUNTER:
		return fmt.Sprintf("%d", int(*m.Metric[0].Counter.Value))
	case dto.MetricType_GAUGE:
		return fmt.Sprintf("%d", int(*m.Metric[0].Gauge.Value))
	case dto.MetricType_HISTOGRAM:
		return fmt.Sprintf("%s", m.Metric[0].Histogram.String())
	}
	return ""
}

func getMetrics(input, outAmdSMI string) ([]byte, map[string]*dto.MetricFamily, []amdSMIMetrics, error) {
	done := make(chan error)

	var content []byte
	exporterMetric := map[string]*dto.MetricFamily{}
	smiMetric := []amdSMIMetrics{}

	go func() {
		resp, err := http.Get(input)
		if err != nil {
			done <- err
			return
		}

		content, err = io.ReadAll(resp.Body)
		if err != nil {
			done <- err
			return
		}

		exporterMetric, err = parseMF(bytes.NewBuffer(content))
		if err != nil {
			done <- err
			return
		}

		done <- resp.Body.Close()
	}()

	go func() {
		// remove old file
		err := os.Remove(outAmdSMI)
		if err != nil {
			//skip err
			if !os.IsNotExist(err) {
				done <- err
				return
			}
		}

		buf, err := exec.Command("amd-smi", "metric", "--json", "--file", outAmdSMI).CombinedOutput()
		if err != nil {
			done <- fmt.Errorf(err.Error() + ": " + string(buf))
			return
		}

		data, err := os.ReadFile(outAmdSMI)
		if err != nil {
			done <- err
			return
		}

		done <- json.Unmarshal(data, &smiMetric)
	}()

	var retErr error
	for i := 0; i < 2; i++ {
		err := <-done
		if err == nil {
			continue
		}
		if retErr == nil {
			retErr = err
			continue
		}
		retErr = fmt.Errorf(retErr.Error() + ": " + err.Error())
	}
	return content, exporterMetric, smiMetric, retErr
}

func compareMetricsLastCurrent(last, current map[string]*dto.MetricFamily) (diff [][3]string) {
	if last == nil {
		return
	}
	existsInCurrNotInLast := []*dto.MetricFamily{}
	diffMetrics := [][]*dto.MetricFamily{}
	for k, currItem := range current {
		lastItem, ok := last[k]
		if !ok {
			existsInCurrNotInLast = append(existsInCurrNotInLast, currItem)
			continue
		}

		delete(last, k)

		if reflect.DeepEqual(lastItem, currItem) {
			continue
		}
		diffMetrics = append(diffMetrics, []*dto.MetricFamily{lastItem, currItem})
	}

	existsInLastNotInCurr := []*dto.MetricFamily{}
	for _, v := range last {
		existsInLastNotInCurr = append(existsInLastNotInCurr, v)
	}
	sort.Slice(existsInLastNotInCurr, func(i, j int) bool {
		return existsInLastNotInCurr[i].GetName() < existsInLastNotInCurr[j].GetName()
	})
	for _, v := range existsInLastNotInCurr {
		diff = append(diff, [3]string{v.GetName(), getValue(v), ""})
	}

	sort.Slice(existsInCurrNotInLast, func(i, j int) bool {
		return existsInCurrNotInLast[i].GetName() < existsInCurrNotInLast[j].GetName()
	})
	for _, v := range existsInCurrNotInLast {
		diff = append(diff, [3]string{v.GetName(), "", getValue(v)})
	}

	sort.Slice(diffMetrics, func(i, j int) bool {
		return diffMetrics[i][0].GetName() < diffMetrics[j][0].GetName()
	})
	for _, v := range diffMetrics {
		diff = append(diff, [3]string{v[0].GetName(), getValue(v[0]), getValue(v[1])})
	}
	return
}

func setCmpMapWithValue(suffix, value string, cmpMap map[string][2]string, m *dto.Metric) {
	var gpuID string
	for _, l := range m.GetLabel() {
		switch l.GetName() {
		case "GPU_ID":
			gpuID = l.GetValue()
		}
	}
	key := "gpu" + gpuID + suffix
	data, ok := cmpMap[key]
	if !ok {
		cmpMap[key] = [2]string{value, ""}
	} else {
		data[0] = value
		cmpMap[key] = data
	}
}

func setCmpMap(suffix string, cmpMap map[string][2]string, m *dto.Metric) {
	setCmpMapWithValue(suffix, strconv.Itoa(int(m.Gauge.GetValue())), cmpMap, m)
}

func setCmpMapBatch(suffix string, cmpMap map[string][2]string, metrics []*dto.Metric) {
	for _, m := range metrics {
		setCmpMap(suffix, cmpMap, m)
	}
}

func createCmpMap(exporter map[string]*dto.MetricFamily, smi []amdSMIMetrics) (map[string][2]string, error) {
	ret := map[string][2]string{}

	//fill data from amd-smi data
	var key string
	for gpuID, metrics := range smi {
		// usage
		key = "gpu" + strconv.Itoa(gpuID) + "_usage_gfx"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Usage.GfxActivity.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_usage_umc"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Usage.UmcActivity.Value)}

		// power
		key = "gpu" + strconv.Itoa(gpuID) + "_socket_power"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Power.SocketPower.Value)}

		// temperature
		key = "gpu" + strconv.Itoa(gpuID) + "_temperature_hotspot"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Temperature.Hotspot.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_temperature_mem"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Temperature.Mem.Value)}
		// clock system
		key = "gpu" + strconv.Itoa(gpuID) + "_clock_sys_0"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Clock.Gfx0.Clk.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_clock_sys_1"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Clock.Gfx1.Clk.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_clock_sys_2"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Clock.Gfx2.Clk.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_clock_sys_3"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Clock.Gfx3.Clk.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_clock_sys_4"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Clock.Gfx4.Clk.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_clock_sys_5"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Clock.Gfx5.Clk.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_clock_sys_6"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Clock.Gfx6.Clk.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_clock_sys_7"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Clock.Gfx7.Clk.Value)}

		// clock memory
		key = "gpu" + strconv.Itoa(gpuID) + "_clock_mem"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Clock.Mem0.Clk.Value)}

		// clock video
		key = "gpu" + strconv.Itoa(gpuID) + "_clock_video_0"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Clock.Vclk0.Clk.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_clock_video_1"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Clock.Vclk1.Clk.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_clock_video_2"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Clock.Vclk2.Clk.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_clock_video_3"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Clock.Vclk3.Clk.Value)}

		// clock data
		key = "gpu" + strconv.Itoa(gpuID) + "_clock_data_0"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Clock.Dclk0.Clk.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_clock_data_1"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Clock.Dclk1.Clk.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_clock_data_2"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Clock.Dclk2.Clk.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_clock_data_3"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Clock.Dclk3.Clk.Value)}

		// pcie
		key = "gpu" + strconv.Itoa(gpuID) + "_pcie_speed"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Pcie.Speed.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_pcie_bandwidth"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Pcie.Bandwidth.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_pcie_replay_count"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Pcie.ReplayCount)}
		key = "gpu" + strconv.Itoa(gpuID) + "_pcie_recovery_count"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Pcie.L0ToRecoveryCount)}
		key = "gpu" + strconv.Itoa(gpuID) + "_pcie_replay_roll_over_count"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Pcie.ReplayRollOverCount)}
		key = "gpu" + strconv.Itoa(gpuID) + "_pcie_nak_sent_count"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Pcie.NakSentCount)}
		key = "gpu" + strconv.Itoa(gpuID) + "_pcie_nak_received_count"
		ret[key] = [2]string{"", strconv.Itoa(metrics.Pcie.NakReceivedCount)}

		// energy
		key = "gpu" + strconv.Itoa(gpuID) + "_total_energy"
		ret[key] = [2]string{"", strconv.Itoa(int(metrics.Energy.TotalEnergyConsumption.Value))}

		// mem usage
		key = "gpu" + strconv.Itoa(gpuID) + "_total_vram"
		ret[key] = [2]string{"", strconv.Itoa(metrics.MemUsage.TotalVram.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_used_vram"
		ret[key] = [2]string{"", strconv.Itoa(metrics.MemUsage.UsedVram.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_free_vram"
		ret[key] = [2]string{"", strconv.Itoa(metrics.MemUsage.FreeVram.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_total_visible_vram"
		ret[key] = [2]string{"", strconv.Itoa(metrics.MemUsage.TotalVisibleVram.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_used_visible_vram"
		ret[key] = [2]string{"", strconv.Itoa(metrics.MemUsage.UsedVisibleVram.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_free_visible_vram"
		ret[key] = [2]string{"", strconv.Itoa(metrics.MemUsage.FreeVisibleVram.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_total_gtt"
		ret[key] = [2]string{"", strconv.Itoa(metrics.MemUsage.TotalGtt.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_used_gtt"
		ret[key] = [2]string{"", strconv.Itoa(metrics.MemUsage.UsedGtt.Value)}
		key = "gpu" + strconv.Itoa(gpuID) + "_free_gtt"
		ret[key] = [2]string{"", strconv.Itoa(metrics.MemUsage.FreeGtt.Value)}
	}

	//fill data from exporter
	var (
		err        error
		clockType  string
		clockIndex int
	)
	_, sysIndexes, videoIndexes, dataIndexes, err := scanMetrics(exporter)
	if err != nil {
		return nil, err
	}

	for k, v := range exporter {
		switch k {
		case "gpu_gfx_activity":
			setCmpMapBatch("_usage_gfx", ret, v.Metric)
		case "gpu_umc_activity":
			setCmpMapBatch("_usage_umc", ret, v.Metric)
		case "gpu_package_power":
			setCmpMapBatch("_socket_power", ret, v.Metric)
		case "gpu_junction_temperature":
			setCmpMapBatch("_temperature_hotspot", ret, v.Metric)
		case "gpu_memory_temperature":
			setCmpMapBatch("_temperature_mem", ret, v.Metric)
		case "pcie_speed":
			setCmpMapBatch("_pcie_speed", ret, v.Metric)
		case "pcie_bandwidth":
			setCmpMapBatch("_pcie_bandwidth", ret, v.Metric)
		case "pcie_recovery_count":
			setCmpMapBatch("_pcie_recovery_count", ret, v.Metric)
		case "pcie_replay_count":
			setCmpMapBatch("_pcie_replay_count", ret, v.Metric)
		case "pcie_replay_rollover_count":
			setCmpMapBatch("_pcie_replay_roll_over_count", ret, v.Metric)
		case "pcie_nack_sent_count":
			setCmpMapBatch("_pcie_nak_sent_count", ret, v.Metric)
		case "pcie_nack_received_count":
			setCmpMapBatch("_pcie_nak_received_count", ret, v.Metric)

			// exporter's unit is uJ, but amd-smi's unit is J, so need convert to compare
		case "gpu_energy_consumed":
			for _, m := range v.Metric {
				setCmpMapWithValue("_total_energy", strconv.Itoa(int(m.Gauge.GetValue()/1000000)), ret, m)
			}

			// mem_usage
		case "gpu_total_vram":
			setCmpMapBatch("_total_vram", ret, v.Metric)
		case "gpu_used_vram":
			setCmpMapBatch("_used_vram", ret, v.Metric)
		case "gpu_free_vram":
			setCmpMapBatch("_free_vram", ret, v.Metric)
		case "gpu_total_visible_vram":
			setCmpMapBatch("_total_visible_vram", ret, v.Metric)
		case "gpu_used_visible_vram":
			setCmpMapBatch("_used_visible_vram", ret, v.Metric)
		case "gpu_free_visible_vram":
			setCmpMapBatch("_free_visible_vram", ret, v.Metric)
		case "gpu_total_gtt":
			setCmpMapBatch("_total_gtt", ret, v.Metric)
		case "gpu_used_gtt":
			setCmpMapBatch("_used_gtt", ret, v.Metric)
		case "gpu_free_gtt":
			setCmpMapBatch("_free_gtt", ret, v.Metric)

		case "gpu_clock":
			for _, m := range v.Metric {
				gpuID := ""
				for _, l := range m.GetLabel() {
					switch l.GetName() {
					case "GPU_ID":
						gpuID = l.GetValue()
					case "clock_type":
						clockType = l.GetValue()
					case "clock_index":
						clockIndex, err = strconv.Atoi(l.GetValue())
						if err != nil {
							return nil, err
						}
					}
				}
				switch clockType {
				case "GPU_CLOCK_TYPE_SYSTEM":
					sys := sysIndexes[gpuID]
					setCmpMap("_clock_sys_"+strconv.Itoa(clockIndex-sys), ret, m)
				case "GPU_CLOCK_TYPE_DATA":
					data := dataIndexes[gpuID]
					setCmpMap("_clock_data_"+strconv.Itoa(clockIndex-data), ret, m)
				case "GPU_CLOCK_TYPE_VIDEO":
					video := videoIndexes[gpuID]
					setCmpMap("_clock_video_"+strconv.Itoa(clockIndex-video), ret, m)
				case "GPU_CLOCK_TYPE_MEMORY":
					setCmpMap("_clock_mem", ret, m)
				}
			}
		}
	}
	return ret, nil
}

func compareMetricsExporterAMDSMI(current map[string]*dto.MetricFamily, smi []amdSMIMetrics) ([][3]string,
	error) {
	metricMap, err := createCmpMap(current, smi)
	if err != nil {
		return nil, err
	}

	diffMetrics := [][3]string{}
	for key, data := range metricMap {
		if data[0] != data[1] {
			diffMetrics = append(diffMetrics, [3]string{key, data[0], data[1]})
		}
	}

	sort.Slice(diffMetrics, func(i, j int) bool {
		return diffMetrics[i][0] < diffMetrics[j][0]
	})

	return diffMetrics, nil
}

func compareMetrics(last, current map[string]*dto.MetricFamily, smi []amdSMIMetrics) (diff [][6]string, err error) {
	diffLastCurrent := compareMetricsLastCurrent(last, current)
	diffExporterSMI, err := compareMetricsExporterAMDSMI(current, smi)
	if err != nil {
		return nil, err
	}

	min := len(diffLastCurrent)
	if min > len(diffExporterSMI) {
		min = len(diffExporterSMI)
	}

	for i := 0; i < min; i++ {
		diff = append(diff, [6]string{diffLastCurrent[i][0], diffLastCurrent[i][1], diffLastCurrent[i][2],
			diffExporterSMI[i][0], diffExporterSMI[i][1], diffExporterSMI[i][2]})
	}

	for i := min; i < len(diffLastCurrent); i++ {
		diff = append(diff, [6]string{diffLastCurrent[i][0], diffLastCurrent[i][1], diffLastCurrent[i][2],
			"", "", ""})
	}
	for i := min; i < len(diffExporterSMI); i++ {
		diff = append(diff, [6]string{"", "", "",
			diffExporterSMI[i][0], diffExporterSMI[i][1], diffExporterSMI[i][2]})
	}
	return
}

func watchMetric(input, outCurr, outLast, outSMI string, interval time.Duration) {
	var (
		last, current map[string]*dto.MetricFamily
		bufCurrent    []byte
		smi           []amdSMIMetrics
		err           error
	)
	if !strings.HasPrefix(input, "http") {
		input = "http://" + input
	}
	for {
		bufCurrent, current, smi, err = getMetrics(input, outSMI)
		if err != nil {
			log.Fatalln(err)
		}

		diff, err := compareMetrics(last, current, smi)
		if err != nil {
			log.Fatalln(err)
		}

		// save output
		if last != nil {
			err = os.Rename(outCurr, outLast)
			if err != nil {
				log.Fatalln(err)
			}
			err = os.WriteFile(outCurr, bufCurrent, 0644)
			if err != nil {
				log.Fatalln(err)
			}
		} else {
			err = os.WriteFile(outCurr, bufCurrent, 0644)
			if err != nil {
				log.Fatalln(err)
			}
		}

		clearTerminal()

		writer := tabwriter.NewWriter(os.Stdout, 5, 1, 1, ' ', 0)

		writer.Write([]byte("\n"))
		writer.Write([]byte("Metric\tLast Iteration\tCurrent Iteration\t|\tMetric\tExporter\tAMD-SMI\n"))
		for _, items := range diff {
			for i, item := range items {
				writer.Write([]byte(item))
				switch i {
				case len(items) - 1:
					writer.Write([]byte("\n"))
				case 2:
					writer.Write([]byte("\t|\t"))
				default:
					writer.Write([]byte("\t"))
				}
			}
		}
		writer.Flush()

		time.Sleep(interval)
		last = current
	}
}

func writeOutput(filename string, data interface{}) error {
	content, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(filename, content, 0644)
}

func process(input, output string) error {
	var reader io.ReadCloser
	_, err := os.Stat(input)
	if err == nil {
		reader, err = os.Open(input)
		if err != nil {
			return err
		}
	} else {
		if !strings.HasPrefix(input, "http") {
			input = "http://" + input
		}
		resp, err := http.Get(input)
		if err != nil {
			return err
		}
		reader = resp.Body
	}
	defer reader.Close()
	mf, err := parseMF(reader)
	if err != nil {
		return err
	}

	values := make([]*dto.MetricFamily, len(mf))
	index := 0
	for _, v := range mf {
		values[index] = v
		index++
	}
	return writeOutput(output, values)
}
