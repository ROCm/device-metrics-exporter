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
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"

	"github.com/ROCm/device-metrics-exporter/pkg/amdnic/gen/nicmetrics"
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

	portStatsOut, err := ExecWithContext("nicctl show port statistics -j")
	if err != nil {
		logger.Log.Printf("failed to get port statistics, err: %+v", err)
		return err
	}

	// Unmarshal the JSON data into the port statistics
	var portStats nicmetrics.PortStatsList
	err = json.Unmarshal(portStatsOut, &portStats)
	if err != nil {
		logger.Log.Printf("error unmarshaling port statistics data: %v", err)
		return err
	}

	// for each reported port stats, find out the port name and report metrics to prometheus
	for _, nic := range portStats.NIC {
		labels := nc.na.populateLabelsFromNIC(nic.ID)
		for _, port := range nic.Port {
			portName := nc.na.nics[nic.ID].GetPortName()
			portID := nc.na.nics[nic.ID].GetPortIndex()
			labels[LabelPortName] = portName
			labels[LabelPortID] = portID

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

			//tx counter
			nc.na.m.nicPortStatsFramesTxOk.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_OK)))
			nc.na.m.nicPortStatsFramesTxAll.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_ALL)))
			nc.na.m.nicPortStatsFramesTxBad.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_BAD)))
			nc.na.m.nicPortStatsFramesTxPause.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_PAUSE)))
			nc.na.m.nicPortStatsFramesTxPripause.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_PRIPAUSE)))
			nc.na.m.nicPortStatsFramesTxLessThan64b.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_LESS_THAN_64B)))
			nc.na.m.nicPortStatsFramesTxTruncated.With(labels).Set(float64(utils.StringToUint64(port.Statistics.FRAMES_TX_TRUNCATED)))
			nc.na.m.nicPortStatsRsfecCorrectableWord.With(labels).Set(float64(utils.StringToUint64(port.Statistics.RSFEC_CORRECTABLE_WORD)))
			nc.na.m.nicPortStatsRsfecChSymbolErrCnt.With(labels).Set(float64(utils.StringToUint64(port.Statistics.RSFEC_CH_SYMBOL_ERR_CNT)))
		}
	}

	return nil
}

func (nc *NICCtlClient) UpdateLifStats(workloads map[string]scheduler.Workload) error {
	if !fetchLifMetrics {
		return nil
	}

	lifStatsOut, err := ExecWithContext("nicctl show lif statistics -j")
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
			labels[LabelLifName] = nc.na.nics[nic.ID].GetLifName(lif.Spec.ID)
			labels[LabelPortName] = nc.na.nics[nic.ID].GetPortName()

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
	if !fetchQPMetrics {
		return nil
	}

	for _, nic := range nc.na.nics {
		for _, lif := range nic.Lifs {
			wg.Add(1)

			go func(lif *Lif) {
				defer wg.Done()

				cmd := fmt.Sprintf("nicctl show rdma queue-pair statistics --lif %s -j", lif.UUID)
				qpLifStatsOut, err := ExecWithContext(cmd)
				if err != nil {
					logger.Log.Printf("error getting QP stats for %s, err: %+v", lif.Name, err)
					return
				}

				var rdmaQPStats nicmetrics.RdmaQPStats
				err = json.Unmarshal(qpLifStatsOut, &rdmaQPStats)
				if err != nil {
					logger.Log.Printf("error unmarshalling QP stats for %s , err: %v", lif.Name, err)
					return
				}

				for _, nic := range rdmaQPStats.NicList {
					labels := nc.na.populateLabelsFromNIC(nic.ID)
					for _, qplif := range nic.LifList {
						workloadLabels := nc.na.getAssociatedWorkloadLabels(nic.ID, qplif.Spec.ID, workloads)
						for k, v := range workloadLabels {
							labels[k] = v
						}
						// Add LIF labels for QP metrics
						labels[LabelLifName] = nc.na.nics[nic.ID].GetLifName(qplif.Spec.ID)

						for _, qp := range qplif.QPStatsList {
							// Add QueuePair ID label
							labels[LabelQPID] = qp.Spec.ID

							nc.na.m.qpSqReqTxNumPackets.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Sq.ReqTx.NUM_PACKET)))
							nc.na.m.qpSqReqTxNumSendMsgsRke.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Sq.ReqTx.NUM_SEND_MSGS_WITH_RKE)))
							nc.na.m.qpSqReqTxNumLocalAckTimeouts.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Sq.ReqTx.NUM_LOCAL_ACK_TIMEOUTS)))
							nc.na.m.qpSqReqTxRnrTimeout.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Sq.ReqTx.RNR_TIMEOUT)))
							nc.na.m.qpSqReqTxTimesSQdrained.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Sq.ReqTx.TIMES_SQ_DRAINED)))
							nc.na.m.qpSqReqTxNumCNPsent.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Sq.ReqTx.NUM_CNP_SENT)))

							nc.na.m.qpSqReqRxNumPackets.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Sq.ReqRx.NUM_PACKET)))
							nc.na.m.qpSqReqRxNumPacketsEcnMarked.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Sq.ReqRx.NUM_PKTS_WITH_ECN_MARKING)))

							if qp.Stats.Sq.DcQcn != nil {
								nc.na.m.qpSqQcnCurrByteCounter.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Sq.DcQcn.CURR_BYTE_COUNTER)))
								nc.na.m.qpSqQcnNumByteCounterExpired.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Sq.DcQcn.NUM_BYTE_COUNTER_EXPIRED)))
								nc.na.m.qpSqQcnNumTimerExpired.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Sq.DcQcn.NUM_TIMER_EXPIRED)))
								nc.na.m.qpSqQcnNumAlphaTimerExpired.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Sq.DcQcn.NUM_ALPHA_TIMER_EXPIRED)))
								nc.na.m.qpSqQcnNumCNPrcvd.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Sq.DcQcn.NUM_CNP_RCVD)))
								nc.na.m.qpSqQcnNumCNPprocessed.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Sq.DcQcn.NUM_CNP_PROCESSED)))
							}

							nc.na.m.qpRqRspTxNumPackets.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.RespTx.NUM_PACKET)))
							nc.na.m.qpRqRspTxRnrError.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.RespTx.RNR_ERROR)))
							nc.na.m.qpRqRspTxNumSequenceError.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.RespTx.NUM_SEQUENCE_ERROR)))
							nc.na.m.qpRqRspTxRPByteThresholdHits.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.RespTx.NUM_RP_BYTE_THRES_HIT)))
							nc.na.m.qpRqRspTxRPMaxRateHits.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.RespTx.NUM_RP_MAX_RATE_HIT)))

							nc.na.m.qpRqRspRxNumPackets.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.RespRx.NUM_PACKET)))
							nc.na.m.qpRqRspRxNumSendMsgsRke.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.RespRx.NUM_SEND_MSGS_WITH_RKE)))
							nc.na.m.qpRqRspRxNumPacketsEcnMarked.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.RespRx.NUM_PKTS_WITH_ECN_MARKING)))
							nc.na.m.qpRqRspRxNumCNPsReceived.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.RespRx.NUM_CNPS_RECEIVED)))
							nc.na.m.qpRqRspRxMaxRecircDrop.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.RespRx.MAX_RECIRC_EXCEEDED_DROP)))
							nc.na.m.qpRqRspRxNumMemWindowInvalid.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.RespRx.NUM_MEM_WINDOW_INVALID)))
							nc.na.m.qpRqRspRxNumDuplWriteSendOpc.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.RespRx.NUM_DUPL_WITH_WR_SEND_OPC)))
							nc.na.m.qpRqRspRxNumDupReadBacktrack.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.RespRx.NUM_DUPL_READ_BACKTRACK)))
							nc.na.m.qpRqRspRxNumDupReadDrop.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.RespRx.NUM_DUPL_READ_ATOMIC_DROP)))

							if qp.Stats.Rq.DcQcn != nil {
								nc.na.m.qpRqQcnCurrByteCounter.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.DcQcn.CURR_BYTE_COUNTER)))
								nc.na.m.qpRqQcnNumByteCounterExpired.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.DcQcn.NUM_BYTE_COUNTER_EXPIRED)))
								nc.na.m.qpRqQcnNumTimerExpired.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.DcQcn.NUM_TIMER_EXPIRED)))
								nc.na.m.qpRqQcnNumAlphaTimerExpired.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.DcQcn.NUM_ALPHA_TIMER_EXPIRED)))
								nc.na.m.qpRqQcnNumCNPrcvd.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.DcQcn.NUM_CNP_RCVD)))
								nc.na.m.qpRqQcnNumCNPprocessed.With(labels).Set(float64(utils.StringToUint64(qp.Stats.Rq.DcQcn.NUM_CNP_PROCESSED)))
							}

						}
					}
				}
			}(lif)
		}
	}
	wg.Wait()
	return nil
}
