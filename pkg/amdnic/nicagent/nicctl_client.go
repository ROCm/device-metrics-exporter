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

package nicagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os/exec"
	"strconv"
	"sync"

	"github.com/ROCm/device-metrics-exporter/pkg/amdnic/gen/nicmetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
)

type NICCtlClient struct {
	sync.Mutex
	na *NICAgentClient
}

func newNICCtlClient(na *NICAgentClient) *NICCtlClient {
	nc := &NICCtlClient{na: na}
	return nc
}

func (nc *NICCtlClient) Init() error {
	nc.Lock()
	defer nc.Unlock()
	// TODO check nicctl connection to NIC cards and return error for failure
	return nil
}

func (nc *NICCtlClient) IsActive() bool {
	nc.Lock()
	defer nc.Unlock()
	if _, err := exec.LookPath(NICCtlBinary); err == nil {
		return true
	}
	return false
}

func (rc *NICCtlClient) GetClientName() string {
	return NICCtlClientName
}

func (nc *NICCtlClient) UpdateNICStats(workloads map[string]scheduler.Workload) error {
	nc.Lock()
	defer nc.Unlock()

	fn_ptrs := []func(map[string]scheduler.Workload) error{
		nc.UpdatePortStats,
		nc.UpdateLifStats,
		nc.UpdateQPStats}

	var wg sync.WaitGroup
	for _, fn := range fn_ptrs {
		wg.Add(1)
		go func(f func(map[string]scheduler.Workload) error) {
			defer wg.Done()
			if err := f(workloads); err != nil {
				logger.Log.Printf("failed to update NIC stats, err: %+v", err)
			}
		}(fn)
	}
	wg.Wait()
	return nil
}

func (nc *NICCtlClient) UpdatePortStats(workloads map[string]scheduler.Workload) error {
	if !fetchPortMetrics {
		return nil
	}

	// Fetch regular port statistics
	portStatsOut, err := ExecWithContext("nicctl show port statistics -j", nc.na.cmdExec)
	if err != nil {
		logger.Log.Printf("failed to get port statistics, err: %+v", err)
		return err
	}

	// Fetch port rate statistics only if rate metrics are enabled
	var portRateStatsOut []byte
	if fetchPortRateMetrics {
		portRateStatsOut, err = ExecWithContext("nicctl show port statistics --rate -j", nc.na.cmdExec)
		if err != nil {
			logger.Log.Printf("failed to get port rate statistics, err: %+v", err)
			// Don't return error - continue with regular stats even if rate stats fail
		}
	}

	// Unmarshal the JSON data into the port statistics
	var portStats nicmetrics.PortStatsList
	err = json.Unmarshal(portStatsOut, &portStats)
	if err != nil {
		logger.Log.Printf("error unmarshaling port statistics data: %v", err)
		return err
	}

	// Unmarshal port rate statistics if available
	var portRateStats nicmetrics.PortStatsList
	rateStatsAvailable := false
	if portRateStatsOut != nil {
		err = json.Unmarshal(portRateStatsOut, &portRateStats)
		if err != nil {
			logger.Log.Printf("error unmarshaling port rate statistics data: %v", err)
		} else {
			rateStatsAvailable = true
		}
	}

	// Create a map for quick rate stats lookup by NIC ID and Port ID
	rateStatsMap := make(map[string]map[string]*nicmetrics.Port)
	if rateStatsAvailable {
		for _, nic := range portRateStats.NIC {
			rateStatsMap[nic.ID] = make(map[string]*nicmetrics.Port)
			for i := range nic.Port {
				if nic.Port[i].Spec != nil {
					rateStatsMap[nic.ID][nic.Port[i].Spec.ID] = nic.Port[i]
				}
			}
		}
	}

	// for each reported port stats, find out the port name and report metrics to prometheus
	for _, nic := range portStats.NIC {
		labels := nc.na.populateLabelsFromNIC(nic.ID)
		for _, port := range nic.Port {
			portName := nc.na.nics[nic.ID].GetPortName()
			portID := nc.na.nics[nic.ID].GetPortIndex()
			labels[LabelPortName] = portName
			labels[LabelPortID] = portID
			labels[LabelPcieBusId] = nc.na.nics[nic.ID].GetPortPcieAddr()

			// rx counters
			nc.na.m.nicPortStatsFramesRxOk.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_OK)))
			nc.na.m.nicPortStatsFramesRxAll.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_ALL)))
			nc.na.m.nicPortStatsFramesRxBadFcs.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_BAD_FCS)))
			nc.na.m.nicPortStatsFramesRxBadAll.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_BAD_ALL)))
			nc.na.m.nicPortStatsFramesRxPause.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_PAUSE)))
			nc.na.m.nicPortStatsFramesRxBadLength.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_BAD_LENGTH)))
			nc.na.m.nicPortStatsFramesRxUndersized.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_UNDERSIZED)))
			nc.na.m.nicPortStatsFramesRxOversized.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_OVERSIZED)))
			nc.na.m.nicPortStatsFramesRxFragments.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_FRAGMENTS)))
			nc.na.m.nicPortStatsFramesRxJabber.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_JABBER)))
			nc.na.m.nicPortStatsFramesRxPripause.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_PRIPAUSE)))
			nc.na.m.nicPortStatsFramesRxStompedCrc.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_STOMPED_CRC)))
			nc.na.m.nicPortStatsFramesRxTooLong.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_TOO_LONG)))
			nc.na.m.nicPortStatsFramesRxDropped.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_DROPPED)))
			nc.na.m.nicPortStatsFramesRxUnicast.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_UNICAST)))
			nc.na.m.nicPortStatsFramesRxMulticast.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_MULTICAST)))
			nc.na.m.nicPortStatsFramesRxBroadcast.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_BROADCAST)))
			nc.na.m.nicPortStatsFramesRxPri0.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_PRI_0)))
			nc.na.m.nicPortStatsFramesRxPri1.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_PRI_1)))
			nc.na.m.nicPortStatsFramesRxPri2.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_PRI_2)))
			nc.na.m.nicPortStatsFramesRxPri3.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_PRI_3)))
			nc.na.m.nicPortStatsFramesRxPri4.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_PRI_4)))
			nc.na.m.nicPortStatsFramesRxPri5.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_PRI_5)))
			nc.na.m.nicPortStatsFramesRxPri6.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_PRI_6)))
			nc.na.m.nicPortStatsFramesRxPri7.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_RX_PRI_7)))
			nc.na.m.nicPortStatsOctetsRxOk.With(labels).Set(float64(utils.StringToUint64(port.Statistics.OCTETS_RX_OK)))
			nc.na.m.nicPortStatsOctetsRxAll.With(labels).Set(float64(utils.StringToUint64(port.Statistics.OCTETS_RX_ALL)))

			//tx counter
			nc.na.m.nicPortStatsFramesTxOk.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_OK)))
			nc.na.m.nicPortStatsFramesTxAll.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_ALL)))
			nc.na.m.nicPortStatsFramesTxBad.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_BAD)))
			nc.na.m.nicPortStatsFramesTxPause.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_PAUSE)))
			nc.na.m.nicPortStatsFramesTxPripause.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_PRIPAUSE)))
			nc.na.m.nicPortStatsFramesTxLessThan64b.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_LESS_THAN_64B)))
			nc.na.m.nicPortStatsFramesTxTruncated.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_TRUNCATED)))
			nc.na.m.nicPortStatsFramesTxUnicast.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_UNICAST)))
			nc.na.m.nicPortStatsFramesTxMulticast.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_MULTICAST)))
			nc.na.m.nicPortStatsFramesTxBroadcast.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_BROADCAST)))
			nc.na.m.nicPortStatsFramesTxPri0.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_PRI_0)))
			nc.na.m.nicPortStatsFramesTxPri1.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_PRI_1)))
			nc.na.m.nicPortStatsFramesTxPri2.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_PRI_2)))
			nc.na.m.nicPortStatsFramesTxPri3.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_PRI_3)))
			nc.na.m.nicPortStatsFramesTxPri4.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_PRI_4)))
			nc.na.m.nicPortStatsFramesTxPri5.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_PRI_5)))
			nc.na.m.nicPortStatsFramesTxPri6.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_PRI_6)))
			nc.na.m.nicPortStatsFramesTxPri7.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_PRI_7)))
			nc.na.m.nicPortStatsOctetsTxOk.With(labels).Set(float64(utils.StringToUint64(port.Statistics.OCTETS_TX_OK)))
			nc.na.m.nicPortStatsOctetsTxAll.With(labels).Set(float64(utils.StringToUint64(port.Statistics.OCTETS_TX_ALL)))
			nc.na.m.nicPortStatsRsfecCorrectableWord.With(labels).Set(float64(utils.StringToUint64(port.Statistics.RSFEC_CORRECTABLE_WORD)))
			nc.na.m.nicPortStatsRsfecUncorrectableWord.With(labels).Set(float64(utils.StringToUint64(port.Statistics.RSFEC_UNCORRECTABLE_WORD)))
			nc.na.m.nicPortStatsRsfecChSymbolErrCnt.With(labels).Set(float64(utils.StringToUint64(port.Statistics.RSFEC_CH_SYMBOL_ERR_CNT)))

			// Add rate statistics if available
			if rateStatsAvailable && port.Spec != nil {
				if ratePort, ok := rateStatsMap[nic.ID][port.Spec.ID]; ok && ratePort.Statistics != nil {
					nc.na.m.nicPortStatsTxPps.With(labels).Set(parseRateValue(ratePort.Statistics.TX_PPS))
					nc.na.m.nicPortStatsTxBps.With(labels).Set(parseRateValue(ratePort.Statistics.TX_BPS))
					nc.na.m.nicPortStatsRxPps.With(labels).Set(parseRateValue(ratePort.Statistics.RX_PPS))
					nc.na.m.nicPortStatsRxBps.With(labels).Set(parseRateValue(ratePort.Statistics.RX_BPS))
				}
			}
		}
	}

	return nil
}

// parseRateValue parses rate strings like "1234 pps", "5678 bps", "3.83 Mpps", "33.16 Gbps"
func parseRateValue(rateStr string) float64 {
	if rateStr == "" {
		// Empty string is abnormal - nicctl should return "0 pps" for zero traffic
		logger.Log.Printf("error: unexpected empty rate string, expected format like '0 pps'")
		return 0.0
	}

	// Split by space to extract the numeric part and unit
	parts := bytes.Fields([]byte(rateStr))
	if len(parts) == 0 {
		logger.Log.Printf("error: rate value has no fields after splitting: %q", rateStr)
		return 0.0
	}

	// Parse the numeric part (may include decimal point)
	numericPart := string(parts[0])
	value, err := strconv.ParseFloat(numericPart, 64)
	if err != nil {
		logger.Log.Printf("error: failed to parse numeric part %q: %v", numericPart, err)
		return 0.0
	}

	// Check for unit multiplier (K, M, G) if there's a second part
	if len(parts) > 1 {
		unit := string(parts[1])
		// Extract multiplier prefix (first character: K, M, G)
		if len(unit) > 0 {
			switch unit[0] {
			case 'K', 'k':
				value *= 1000 // Kilo
			case 'M', 'm':
				value *= 1000000 // Mega
			case 'G', 'g':
				value *= 1000000000 // Giga
				// No multiplier for base units (pps, bps)
			}
		}
	}

	return value
}

func (nc *NICCtlClient) UpdateLifStats(workloads map[string]scheduler.Workload) error {
	if !fetchLifMetrics {
		return nil
	}

	lifStatsOut, err := ExecWithContext("nicctl show lif statistics -j", nc.na.cmdExec)
	if err != nil {
		logger.Log.Printf("failed to get lif statistics, err: %+v", err)
		return err
	}

	var lifStats nicmetrics.LifStatsList
	err = json.Unmarshal(lifStatsOut, &lifStats)
	if err != nil {
		logger.Log.Printf("error unmarshalling lif statistics data, err: %v", err)
		return err
	}

	// filter/fetch only stats that nicagent is interested
	for _, nic := range lifStats.NIC {
		labels := nc.na.populateLabelsFromNIC(nic.ID)
		for _, lif := range nic.Lif {
			workloadLabels := nc.na.getAssociatedWorkloadLabels(nic.ID, lif.Spec.ID, workloads)
			for k, v := range workloadLabels {
				labels[k] = v
			}
			// Add additional labels for NIC metrics
			labels[LabelEthIntfName] = nc.na.nics[nic.ID].GetLifName(lif.Spec.ID)
			labels[LabelPortName] = nc.na.nics[nic.ID].GetPortName()
			labels[LabelPcieBusId] = nc.na.nics[nic.ID].GetLifPcieAddr(lif.Spec.ID)

			// rx counters
			nc.na.m.nicLifStatsRxUnicastPackets.With(labels).Set(float64(utils.StringToUint64(lif.Statistics.RX_UNICAST_PACKETS)))
			nc.na.m.nicLifStatsRxUnicastDropPackets.With(labels).Set(float64(utils.StringToUint64(lif.Statistics.RX_UNICAST_DROP_PACKETS)))
			nc.na.m.nicLifStatsRxMulticastDropPackets.With(labels).Set(float64(utils.StringToUint64(lif.Statistics.RX_MULTICAST_DROP_PACKETS)))
			nc.na.m.nicLifStatsRxBroadcastDropPackets.With(labels).Set(float64(utils.StringToUint64(lif.Statistics.RX_BROADCAST_DROP_PACKETS)))
			nc.na.m.nicLifStatsRxDMAErrors.With(labels).Set(float64(utils.StringToUint64(lif.Statistics.RX_DMA_ERRORS)))

			// tx counters
			nc.na.m.nicLifStatsTxUnicastPackets.With(labels).Set(float64(utils.StringToUint64(lif.Statistics.TX_UNICAST_PACKETS)))
			nc.na.m.nicLifStatsTxUnicastDropPackets.With(labels).Set(float64(utils.StringToUint64(lif.Statistics.TX_UNICAST_DROP_PACKETS)))
			nc.na.m.nicLifStatsTxMulticastDropPackets.With(labels).Set(float64(utils.StringToUint64(lif.Statistics.TX_MULTICAST_DROP_PACKETS)))
			nc.na.m.nicLifStatsTxBroadcastDropPackets.With(labels).Set(float64(utils.StringToUint64(lif.Statistics.TX_BROADCAST_DROP_PACKETS)))
			nc.na.m.nicLifStatsTxDMAErrors.With(labels).Set(float64(utils.StringToUint64(lif.Statistics.TX_DMA_ERRORS)))
		}
	}
	return nil
}

func (nc *NICCtlClient) UpdateQPStats(workloads map[string]scheduler.Workload) error {
	var wg sync.WaitGroup
	if debugMode != globals.DebugModeQP && !fetchQPMetrics && !fetchLIFAggQPMetrics {
		// QP metrics NOT enabled, skip fetching QP stats to save resources
		return nil
	}

	for _, nic := range nc.na.nics {
		wg.Add(1)

		go func(nic *NIC) {
			defer wg.Done()

			cmd := fmt.Sprintf("nicctl show rdma queue-pair statistics --card %s -j", nic.UUID)
			qpLifStatsOut, err := ExecWithContextTimeout(cmd, longCmdTimeout, nc.na.cmdExec)
			if err != nil {
				logger.Log.Printf("error getting QP stats for %s, err: %+v", nic.UUID, err)
				return
			}

			// Replace empty string with empty array for proper unmarshalling
			qpLifStatsOut = bytes.ReplaceAll(qpLifStatsOut, []byte(`"queue_pair": ""`), []byte(`"queue_pair": []`))

			var rdmaQPStats nicmetrics.RdmaQPStats
			err = json.Unmarshal(qpLifStatsOut, &rdmaQPStats)
			if err != nil {
				logger.Log.Printf("error unmarshalling QP stats for %s , err: %v", nic.UUID, err)
				return
			}

			type lifAggregates struct {
				sqReqTxNumPackets          float64
				sqReqTxNumSendMsgsRke      float64
				sqReqTxNumLocalAckTimeouts float64
				sqReqTxRnrTimeout          float64
				sqReqTxTimesSQdrained      float64
				sqReqTxNumCNPsent          float64
				sqReqRxNumPackets          float64
				sqReqRxNumPacketsEcnMarked float64
				sqQcnCurrByteCounter       float64
				sqQcnNumByteCounterExpired float64
				sqQcnNumTimerExpired       float64
				sqQcnNumAlphaTimerExpired  float64
				sqQcnNumCNPrcvd            float64
				sqQcnNumCNPprocessed       float64
				rqRspTxNumPackets          float64
				rqRspTxRnrError            float64
				rqRspTxNumSequenceError    float64
				rqRspTxRPByteThresholdHits float64
				rqRspTxRPMaxRateHits       float64
				rqRspRxNumPackets          float64
				rqRspRxNumSendMsgsRke      float64
				rqRspRxNumPacketsEcnMarked float64
				rqRspRxNumCNPsReceived     float64
				rqRspRxMaxRecircDrop       float64
				rqRspRxNumMemWindowInvalid float64
				rqRspRxNumDuplWriteSendOpc float64
				rqRspRxNumDupReadBacktrack float64
				rqRspRxNumDupReadDrop      float64
				rqQcnCurrByteCounter       float64
				rqQcnNumByteCounterExpired float64
				rqQcnNumTimerExpired       float64
				rqQcnNumAlphaTimerExpired  float64
				rqQcnNumCNPrcvd            float64
				rqQcnNumCNPprocessed       float64
			}

			for _, statsNIC := range rdmaQPStats.NicList {
				nicLabels := nc.na.populateLabelsFromNIC(statsNIC.ID)
				for _, qplif := range statsNIC.LifList {
					// Clone base NIC labels for each LIF to avoid label pollution across LIF iterations
					lifQPLabels := maps.Clone(nicLabels)

					workloadLabels := nc.na.getAssociatedWorkloadLabels(statsNIC.ID, qplif.Spec.ID, workloads)
					for k, v := range workloadLabels {
						lifQPLabels[k] = v
					}
					// Add LIF labels for QP metrics
					lifQPLabels[LabelEthIntfName] = nc.na.nics[statsNIC.ID].GetLifName(qplif.Spec.ID)
					lifQPLabels[LabelPcieBusId] = nc.na.nics[statsNIC.ID].GetLifPcieAddr(qplif.Spec.ID)

					// Create a copy of lifQPLabels for per-QP metrics (will add qp_id below)
					var labels map[string]string
					if debugMode == globals.DebugModeQP || fetchQPMetrics {
						labels = maps.Clone(lifQPLabels)
					}

					// LIF-level aggregation accumulators (sum of all QPs for this LIF)
					var lifAgg lifAggregates

					for _, qp := range qplif.QPStatsList {
						// Parse each field once and reuse for both per-QP export and LIF aggregation
						// SQ Requester Tx
						sqReqTxNumPacket := float64(utils.StringToUint64(qp.Stats.Sq.ReqTx.NUM_PACKET))
						sqReqTxNumSendMsgsRke := float64(utils.StringToUint64(qp.Stats.Sq.ReqTx.NUM_SEND_MSGS_WITH_RKE))
						sqReqTxNumLocalAckTimeouts := float64(utils.StringToUint64(qp.Stats.Sq.ReqTx.NUM_LOCAL_ACK_TIMEOUTS))
						sqReqTxRnrTimeout := float64(utils.StringToUint64(qp.Stats.Sq.ReqTx.RNR_TIMEOUT))
						sqReqTxTimesSQdrained := float64(utils.StringToUint64(qp.Stats.Sq.ReqTx.TIMES_SQ_DRAINED))
						sqReqTxNumCNPsent := float64(utils.StringToUint64(qp.Stats.Sq.ReqTx.NUM_CNP_SENT))

						// SQ Requester Rx
						sqReqRxNumPacket := float64(utils.StringToUint64(qp.Stats.Sq.ReqRx.NUM_PACKET))
						sqReqRxNumPacketsEcnMarked := float64(utils.StringToUint64(qp.Stats.Sq.ReqRx.NUM_PKTS_WITH_ECN_MARKING))

						// RQ Responder Tx
						rqRspTxNumPacket := float64(utils.StringToUint64(qp.Stats.Rq.RespTx.NUM_PACKET))
						rqRspTxRnrError := float64(utils.StringToUint64(qp.Stats.Rq.RespTx.RNR_ERROR))
						rqRspTxNumSequenceError := float64(utils.StringToUint64(qp.Stats.Rq.RespTx.NUM_SEQUENCE_ERROR))
						rqRspTxRPByteThresholdHits := float64(utils.StringToUint64(qp.Stats.Rq.RespTx.NUM_RP_BYTE_THRES_HIT))
						rqRspTxRPMaxRateHits := float64(utils.StringToUint64(qp.Stats.Rq.RespTx.NUM_RP_MAX_RATE_HIT))

						// RQ Responder Rx
						rqRspRxNumPacket := float64(utils.StringToUint64(qp.Stats.Rq.RespRx.NUM_PACKET))
						rqRspRxNumSendMsgsRke := float64(utils.StringToUint64(qp.Stats.Rq.RespRx.NUM_SEND_MSGS_WITH_RKE))
						rqRspRxNumPacketsEcnMarked := float64(utils.StringToUint64(qp.Stats.Rq.RespRx.NUM_PKTS_WITH_ECN_MARKING))
						rqRspRxNumCNPsReceived := float64(utils.StringToUint64(qp.Stats.Rq.RespRx.NUM_CNPS_RECEIVED))
						rqRspRxMaxRecircDrop := float64(utils.StringToUint64(qp.Stats.Rq.RespRx.MAX_RECIRC_EXCEEDED_DROP))
						rqRspRxNumMemWindowInvalid := float64(utils.StringToUint64(qp.Stats.Rq.RespRx.NUM_MEM_WINDOW_INVALID))
						rqRspRxNumDuplWriteSendOpc := float64(utils.StringToUint64(qp.Stats.Rq.RespRx.NUM_DUPL_WITH_WR_SEND_OPC))
						rqRspRxNumDupReadBacktrack := float64(utils.StringToUint64(qp.Stats.Rq.RespRx.NUM_DUPL_READ_BACKTRACK))
						rqRspRxNumDupReadDrop := float64(utils.StringToUint64(qp.Stats.Rq.RespRx.NUM_DUPL_READ_ATOMIC_DROP))

						var sqQcnCurrByteCounter, sqQcnNumByteCounterExpired, sqQcnNumTimerExpired, sqQcnNumAlphaTimerExpired, sqQcnNumCNPrcvd, sqQcnNumCNPprocessed float64
						var rqQcnCurrByteCounter, rqQcnNumByteCounterExpired, rqQcnNumTimerExpired, rqQcnNumAlphaTimerExpired, rqQcnNumCNPrcvd, rqQcnNumCNPprocessed float64

						if qp.Stats.Sq.DcQcn != nil {
							sqQcnCurrByteCounter = float64(utils.StringToUint64(qp.Stats.Sq.DcQcn.CURR_BYTE_COUNTER))
							sqQcnNumByteCounterExpired = float64(utils.StringToUint64(qp.Stats.Sq.DcQcn.NUM_BYTE_COUNTER_EXPIRED))
							sqQcnNumTimerExpired = float64(utils.StringToUint64(qp.Stats.Sq.DcQcn.NUM_TIMER_EXPIRED))
							sqQcnNumAlphaTimerExpired = float64(utils.StringToUint64(qp.Stats.Sq.DcQcn.NUM_ALPHA_TIMER_EXPIRED))
							sqQcnNumCNPrcvd = float64(utils.StringToUint64(qp.Stats.Sq.DcQcn.NUM_CNP_RCVD))
							sqQcnNumCNPprocessed = float64(utils.StringToUint64(qp.Stats.Sq.DcQcn.NUM_CNP_PROCESSED))
						}
						if qp.Stats.Rq.DcQcn != nil {
							rqQcnCurrByteCounter = float64(utils.StringToUint64(qp.Stats.Rq.DcQcn.CURR_BYTE_COUNTER))
							rqQcnNumByteCounterExpired = float64(utils.StringToUint64(qp.Stats.Rq.DcQcn.NUM_BYTE_COUNTER_EXPIRED))
							rqQcnNumTimerExpired = float64(utils.StringToUint64(qp.Stats.Rq.DcQcn.NUM_TIMER_EXPIRED))
							rqQcnNumAlphaTimerExpired = float64(utils.StringToUint64(qp.Stats.Rq.DcQcn.NUM_ALPHA_TIMER_EXPIRED))
							rqQcnNumCNPrcvd = float64(utils.StringToUint64(qp.Stats.Rq.DcQcn.NUM_CNP_RCVD))
							rqQcnNumCNPprocessed = float64(utils.StringToUint64(qp.Stats.Rq.DcQcn.NUM_CNP_PROCESSED))
						}

						// Export per-QP metrics
						if debugMode == globals.DebugModeQP || fetchQPMetrics {
							// Add QueuePair ID label
							labels[LabelQPID] = qp.Spec.ID

							nc.na.m.qpSqReqTxNumPackets.With(labels).Set(sqReqTxNumPacket)
							nc.na.m.qpSqReqTxNumSendMsgsRke.With(labels).Set(sqReqTxNumSendMsgsRke)
							nc.na.m.qpSqReqTxNumLocalAckTimeouts.With(labels).Set(sqReqTxNumLocalAckTimeouts)
							nc.na.m.qpSqReqTxRnrTimeout.With(labels).Set(sqReqTxRnrTimeout)
							nc.na.m.qpSqReqTxTimesSQdrained.With(labels).Set(sqReqTxTimesSQdrained)
							nc.na.m.qpSqReqTxNumCNPsent.With(labels).Set(sqReqTxNumCNPsent)

							nc.na.m.qpSqReqRxNumPackets.With(labels).Set(sqReqRxNumPacket)
							nc.na.m.qpSqReqRxNumPacketsEcnMarked.With(labels).Set(sqReqRxNumPacketsEcnMarked)

							// Only export SQ DCQCN metrics when DcQcn stats are present
							if qp.Stats.Sq.DcQcn != nil {
								nc.na.m.qpSqQcnCurrByteCounter.With(labels).Set(sqQcnCurrByteCounter)
								nc.na.m.qpSqQcnNumByteCounterExpired.With(labels).Set(sqQcnNumByteCounterExpired)
								nc.na.m.qpSqQcnNumTimerExpired.With(labels).Set(sqQcnNumTimerExpired)
								nc.na.m.qpSqQcnNumAlphaTimerExpired.With(labels).Set(sqQcnNumAlphaTimerExpired)
								nc.na.m.qpSqQcnNumCNPrcvd.With(labels).Set(sqQcnNumCNPrcvd)
								nc.na.m.qpSqQcnNumCNPprocessed.With(labels).Set(sqQcnNumCNPprocessed)
							}

							nc.na.m.qpRqRspTxNumPackets.With(labels).Set(rqRspTxNumPacket)
							nc.na.m.qpRqRspTxRnrError.With(labels).Set(rqRspTxRnrError)
							nc.na.m.qpRqRspTxNumSequenceError.With(labels).Set(rqRspTxNumSequenceError)
							nc.na.m.qpRqRspTxRPByteThresholdHits.With(labels).Set(rqRspTxRPByteThresholdHits)
							nc.na.m.qpRqRspTxRPMaxRateHits.With(labels).Set(rqRspTxRPMaxRateHits)

							nc.na.m.qpRqRspRxNumPackets.With(labels).Set(rqRspRxNumPacket)
							nc.na.m.qpRqRspRxNumSendMsgsRke.With(labels).Set(rqRspRxNumSendMsgsRke)
							nc.na.m.qpRqRspRxNumPacketsEcnMarked.With(labels).Set(rqRspRxNumPacketsEcnMarked)
							nc.na.m.qpRqRspRxNumCNPsReceived.With(labels).Set(rqRspRxNumCNPsReceived)
							nc.na.m.qpRqRspRxMaxRecircDrop.With(labels).Set(rqRspRxMaxRecircDrop)
							nc.na.m.qpRqRspRxNumMemWindowInvalid.With(labels).Set(rqRspRxNumMemWindowInvalid)
							nc.na.m.qpRqRspRxNumDuplWriteSendOpc.With(labels).Set(rqRspRxNumDuplWriteSendOpc)
							nc.na.m.qpRqRspRxNumDupReadBacktrack.With(labels).Set(rqRspRxNumDupReadBacktrack)
							nc.na.m.qpRqRspRxNumDupReadDrop.With(labels).Set(rqRspRxNumDupReadDrop)

							// Only export RQ DCQCN metrics when DcQcn stats are present
							if qp.Stats.Rq.DcQcn != nil {
								nc.na.m.qpRqQcnCurrByteCounter.With(labels).Set(rqQcnCurrByteCounter)
								nc.na.m.qpRqQcnNumByteCounterExpired.With(labels).Set(rqQcnNumByteCounterExpired)
								nc.na.m.qpRqQcnNumTimerExpired.With(labels).Set(rqQcnNumTimerExpired)
								nc.na.m.qpRqQcnNumAlphaTimerExpired.With(labels).Set(rqQcnNumAlphaTimerExpired)
								nc.na.m.qpRqQcnNumCNPrcvd.With(labels).Set(rqQcnNumCNPrcvd)
								nc.na.m.qpRqQcnNumCNPprocessed.With(labels).Set(rqQcnNumCNPprocessed)
							}

						}

						// Accumulate values for LIF-aggregated metrics
						if fetchLIFAggQPMetrics {
							lifAgg.sqReqTxNumPackets += sqReqTxNumPacket
							lifAgg.sqReqTxNumSendMsgsRke += sqReqTxNumSendMsgsRke
							lifAgg.sqReqTxNumLocalAckTimeouts += sqReqTxNumLocalAckTimeouts
							lifAgg.sqReqTxRnrTimeout += sqReqTxRnrTimeout
							lifAgg.sqReqTxTimesSQdrained += sqReqTxTimesSQdrained
							lifAgg.sqReqTxNumCNPsent += sqReqTxNumCNPsent

							lifAgg.sqReqRxNumPackets += sqReqRxNumPacket
							lifAgg.sqReqRxNumPacketsEcnMarked += sqReqRxNumPacketsEcnMarked

							lifAgg.sqQcnCurrByteCounter += sqQcnCurrByteCounter
							lifAgg.sqQcnNumByteCounterExpired += sqQcnNumByteCounterExpired
							lifAgg.sqQcnNumTimerExpired += sqQcnNumTimerExpired
							lifAgg.sqQcnNumAlphaTimerExpired += sqQcnNumAlphaTimerExpired
							lifAgg.sqQcnNumCNPrcvd += sqQcnNumCNPrcvd
							lifAgg.sqQcnNumCNPprocessed += sqQcnNumCNPprocessed

							lifAgg.rqRspTxNumPackets += rqRspTxNumPacket
							lifAgg.rqRspTxRnrError += rqRspTxRnrError
							lifAgg.rqRspTxNumSequenceError += rqRspTxNumSequenceError
							lifAgg.rqRspTxRPByteThresholdHits += rqRspTxRPByteThresholdHits
							lifAgg.rqRspTxRPMaxRateHits += rqRspTxRPMaxRateHits

							lifAgg.rqRspRxNumPackets += rqRspRxNumPacket
							lifAgg.rqRspRxNumSendMsgsRke += rqRspRxNumSendMsgsRke
							lifAgg.rqRspRxNumPacketsEcnMarked += rqRspRxNumPacketsEcnMarked
							lifAgg.rqRspRxNumCNPsReceived += rqRspRxNumCNPsReceived
							lifAgg.rqRspRxMaxRecircDrop += rqRspRxMaxRecircDrop
							lifAgg.rqRspRxNumMemWindowInvalid += rqRspRxNumMemWindowInvalid
							lifAgg.rqRspRxNumDuplWriteSendOpc += rqRspRxNumDuplWriteSendOpc
							lifAgg.rqRspRxNumDupReadBacktrack += rqRspRxNumDupReadBacktrack
							lifAgg.rqRspRxNumDupReadDrop += rqRspRxNumDupReadDrop

							lifAgg.rqQcnCurrByteCounter += rqQcnCurrByteCounter
							lifAgg.rqQcnNumByteCounterExpired += rqQcnNumByteCounterExpired
							lifAgg.rqQcnNumTimerExpired += rqQcnNumTimerExpired
							lifAgg.rqQcnNumAlphaTimerExpired += rqQcnNumAlphaTimerExpired
							lifAgg.rqQcnNumCNPrcvd += rqQcnNumCNPrcvd
							lifAgg.rqQcnNumCNPprocessed += rqQcnNumCNPprocessed

						}
					}

					// Export LIF-aggregated metrics
					if fetchLIFAggQPMetrics {
						nc.na.m.lifQpSqReqTxNumPacketTotal.With(lifQPLabels).Set(lifAgg.sqReqTxNumPackets)
						nc.na.m.lifQpSqReqTxNumSendMsgsWithRkeTotal.With(lifQPLabels).Set(lifAgg.sqReqTxNumSendMsgsRke)
						nc.na.m.lifQpSqReqTxNumLocalAckTimeoutsTotal.With(lifQPLabels).Set(lifAgg.sqReqTxNumLocalAckTimeouts)
						nc.na.m.lifQpSqReqTxRnrTimeoutTotal.With(lifQPLabels).Set(lifAgg.sqReqTxRnrTimeout)
						nc.na.m.lifQpSqReqTxTimesSQdrainedTotal.With(lifQPLabels).Set(lifAgg.sqReqTxTimesSQdrained)
						nc.na.m.lifQpSqReqTxNumCNPsentTotal.With(lifQPLabels).Set(lifAgg.sqReqTxNumCNPsent)

						nc.na.m.lifQpSqReqRxNumPacketTotal.With(lifQPLabels).Set(lifAgg.sqReqRxNumPackets)
						nc.na.m.lifQpSqReqRxNumPktsWithEcnMarkingTotal.With(lifQPLabels).Set(lifAgg.sqReqRxNumPacketsEcnMarked)

						nc.na.m.lifQpSqQcnCurrByteCounterTotal.With(lifQPLabels).Set(lifAgg.sqQcnCurrByteCounter)
						nc.na.m.lifQpSqQcnNumByteCounterExpiredTotal.With(lifQPLabels).Set(lifAgg.sqQcnNumByteCounterExpired)
						nc.na.m.lifQpSqQcnNumTimerExpiredTotal.With(lifQPLabels).Set(lifAgg.sqQcnNumTimerExpired)
						nc.na.m.lifQpSqQcnNumAlphaTimerExpiredTotal.With(lifQPLabels).Set(lifAgg.sqQcnNumAlphaTimerExpired)
						nc.na.m.lifQpSqQcnNumCNPrcvdTotal.With(lifQPLabels).Set(lifAgg.sqQcnNumCNPrcvd)
						nc.na.m.lifQpSqQcnNumCNPprocessedTotal.With(lifQPLabels).Set(lifAgg.sqQcnNumCNPprocessed)

						nc.na.m.lifQpRqRspTxNumPacketTotal.With(lifQPLabels).Set(lifAgg.rqRspTxNumPackets)
						nc.na.m.lifQpRqRspTxRnrErrorTotal.With(lifQPLabels).Set(lifAgg.rqRspTxRnrError)
						nc.na.m.lifQpRqRspTxNumSequenceErrorTotal.With(lifQPLabels).Set(lifAgg.rqRspTxNumSequenceError)
						nc.na.m.lifQpRqRspTxNumRpByteThresHitTotal.With(lifQPLabels).Set(lifAgg.rqRspTxRPByteThresholdHits)
						nc.na.m.lifQpRqRspTxNumRpMaxRateHitTotal.With(lifQPLabels).Set(lifAgg.rqRspTxRPMaxRateHits)

						nc.na.m.lifQpRqRspRxNumPacketTotal.With(lifQPLabels).Set(lifAgg.rqRspRxNumPackets)
						nc.na.m.lifQpRqRspRxNumSendMsgsWithRkeTotal.With(lifQPLabels).Set(lifAgg.rqRspRxNumSendMsgsRke)
						nc.na.m.lifQpRqRspRxNumPktsWithEcnMarkingTotal.With(lifQPLabels).Set(lifAgg.rqRspRxNumPacketsEcnMarked)
						nc.na.m.lifQpRqRspRxNumCNPsReceivedTotal.With(lifQPLabels).Set(lifAgg.rqRspRxNumCNPsReceived)
						nc.na.m.lifQpRqRspRxMaxRecircExceededDropTotal.With(lifQPLabels).Set(lifAgg.rqRspRxMaxRecircDrop)
						nc.na.m.lifQpRqRspRxNumMemWindowInvalidTotal.With(lifQPLabels).Set(lifAgg.rqRspRxNumMemWindowInvalid)
						nc.na.m.lifQpRqRspRxNumDuplWithWrSendOpcTotal.With(lifQPLabels).Set(lifAgg.rqRspRxNumDuplWriteSendOpc)
						nc.na.m.lifQpRqRspRxNumDuplReadBacktrackTotal.With(lifQPLabels).Set(lifAgg.rqRspRxNumDupReadBacktrack)
						nc.na.m.lifQpRqRspRxNumDuplReadAtomicDropTotal.With(lifQPLabels).Set(lifAgg.rqRspRxNumDupReadDrop)

						nc.na.m.lifQpRqQcnCurrByteCounterTotal.With(lifQPLabels).Set(lifAgg.rqQcnCurrByteCounter)
						nc.na.m.lifQpRqQcnNumByteCounterExpiredTotal.With(lifQPLabels).Set(lifAgg.rqQcnNumByteCounterExpired)
						nc.na.m.lifQpRqQcnNumTimerExpiredTotal.With(lifQPLabels).Set(lifAgg.rqQcnNumTimerExpired)
						nc.na.m.lifQpRqQcnNumAlphaTimerExpiredTotal.With(lifQPLabels).Set(lifAgg.rqQcnNumAlphaTimerExpired)
						nc.na.m.lifQpRqQcnNumCNPrcvdTotal.With(lifQPLabels).Set(lifAgg.rqQcnNumCNPrcvd)
						nc.na.m.lifQpRqQcnNumCNPprocessedTotal.With(lifQPLabels).Set(lifAgg.rqQcnNumCNPprocessed)
					}
				}
			}
		}(nic)

	}
	wg.Wait()
	return nil
}
