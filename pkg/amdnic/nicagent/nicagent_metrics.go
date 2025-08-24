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
	"strings"

	"github.com/ROCm/device-metrics-exporter/pkg/exporter/gen/exportermetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/globals"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/utils"
	"github.com/prometheus/client_golang/prometheus"
)

// local variables
var (
	mandatoryLables = []string{
		exportermetrics.NICMetricLabel_NIC_UUID.String(),
		exportermetrics.NICMetricLabel_NIC_ID.String(),
		exportermetrics.NICMetricLabel_NIC_SERIAL_NUMBER.String(),
		exportermetrics.NICMetricLabel_NIC_HOSTNAME.String(),
	}
	exportLabels      map[string]bool
	exportFieldMap    map[string]bool
	fieldMetricsMap   []prometheus.Collector
	customLabelMap    map[string]string
	extraPodLabelsMap map[string]string
	k8PodLabelsMap    map[string]map[string]string
)

type metrics struct {
	nicNodesTotal prometheus.GaugeVec
	nicMaxSpeed   prometheus.GaugeVec

	// Port stats
	nicPortStatsFramesRxBadFcs       prometheus.GaugeVec
	nicPortStatsFramesRxBadAll       prometheus.GaugeVec
	nicPortStatsFramesRxPause        prometheus.GaugeVec
	nicPortStatsFramesRxBadLength    prometheus.GaugeVec
	nicPortStatsFramesRxUndersized   prometheus.GaugeVec
	nicPortStatsFramesRxOversized    prometheus.GaugeVec
	nicPortStatsFramesRxFragments    prometheus.GaugeVec
	nicPortStatsFramesRxJabber       prometheus.GaugeVec
	nicPortStatsFramesRxPripause     prometheus.GaugeVec
	nicPortStatsFramesRxStompedCrc   prometheus.GaugeVec
	nicPortStatsFramesRxTooLong      prometheus.GaugeVec
	nicPortStatsFramesRxDropped      prometheus.GaugeVec
	nicPortStatsFramesTxBad          prometheus.GaugeVec
	nicPortStatsFramesTxPause        prometheus.GaugeVec
	nicPortStatsFramesTxPripause     prometheus.GaugeVec
	nicPortStatsFramesTxLessThan64b  prometheus.GaugeVec
	nicPortStatsFramesTxTruncated    prometheus.GaugeVec
	nicPortStatsRsfecCorrectableWord prometheus.GaugeVec
	nicPortStatsRsfecChSymbolErrCnt  prometheus.GaugeVec

	//RDMA Stats
	rdmaTxUcastPkts prometheus.GaugeVec
	rdmaTxCnpPkts   prometheus.GaugeVec
	rdmaRxUcastPkts prometheus.GaugeVec
	rdmaRxCnpPkts   prometheus.GaugeVec
	rdmaRxEcnPkts   prometheus.GaugeVec
	//RDMA Req Rx Stats
	rdmaReqRxPktSeqErr     prometheus.GaugeVec
	rdmaReqRxRnrRetryErr   prometheus.GaugeVec
	rdmaReqRxRmtAccErr     prometheus.GaugeVec
	rdmaReqRxRmtReqErr     prometheus.GaugeVec
	rdmaReqRxOperErr       prometheus.GaugeVec
	rdmaReqRxImplNakSeqErr prometheus.GaugeVec
	rdmaReqRxCqeErr        prometheus.GaugeVec
	rdmaReqRxCqeFlush      prometheus.GaugeVec
	rdmaReqRxDupResp       prometheus.GaugeVec
	rdmaReqRxInvalidPkts   prometheus.GaugeVec
	//RDMA Req Tx Stats
	rdmaReqTxLocErr       prometheus.GaugeVec
	rdmaReqTxLocOperErr   prometheus.GaugeVec
	rdmaReqTxMemMgmtErr   prometheus.GaugeVec
	rdmaReqTxRetryExcdErr prometheus.GaugeVec
	rdmaReqTxLocSglInvErr prometheus.GaugeVec
	//RDMA Resp Rx Stats
	rdmaRespRxDupRequest     prometheus.GaugeVec
	rdmaRespRxOutofBuf       prometheus.GaugeVec
	rdmaRespRxOutoufSeq      prometheus.GaugeVec
	rdmaRespRxCqeErr         prometheus.GaugeVec
	rdmaRespRxCqeFlush       prometheus.GaugeVec
	rdmaRespRxLocLenErr      prometheus.GaugeVec
	rdmaRespRxInvalidRequest prometheus.GaugeVec
	rdmaRespRxLocOperErr     prometheus.GaugeVec
	rdmaRespRxOutofAtomic    prometheus.GaugeVec
	rdmaRespRxS0TableErr     prometheus.GaugeVec
	//RDMA Resp Tx Stats
	rdmaRespTxPktSeqErr      prometheus.GaugeVec
	rdmaRespTxRmtInvalReqErr prometheus.GaugeVec
	rdmaRespTxRmtAccErr      prometheus.GaugeVec
	rdmaRespTxRmtOperErr     prometheus.GaugeVec
	rdmaRespTxRnrRetryErr    prometheus.GaugeVec
	rdmaRespTxLocSglInvErr   prometheus.GaugeVec

	//LifStats
	nicLifStatsRxUnicastPackets       prometheus.GaugeVec
	nicLifStatsRxUnicastDropPackets   prometheus.GaugeVec
	nicLifStatsRxMulticastDropPackets prometheus.GaugeVec
	nicLifStatsRxBroadcastDropPackets prometheus.GaugeVec
	nicLifStatsRxDMAErrors            prometheus.GaugeVec
	nicLifStatsTxUnicastPackets       prometheus.GaugeVec
	nicLifStatsTxUnicastDropPackets   prometheus.GaugeVec
	nicLifStatsTxMulticastDropPackets prometheus.GaugeVec
	nicLifStatsTxBroadcastDropPackets prometheus.GaugeVec
	nicLifStatsTxDMAErrors            prometheus.GaugeVec

	//QPStats
	//QPStats Requester TX
	qpSqReqTxNumPackets          prometheus.GaugeVec
	qpSqReqTxNumSendMsgsRke      prometheus.GaugeVec
	qpSqReqTxNumLocalAckTimeouts prometheus.GaugeVec
	qpSqReqTxRnrTimeout          prometheus.GaugeVec
	qpSqReqTxTimesSQdrained      prometheus.GaugeVec
	qpSqReqTxNumCNPsent          prometheus.GaugeVec
	//QPStats Requester RX
	qpSqReqRxNumPackets          prometheus.GaugeVec
	qpSqReqRxNumPacketsEcnMarked prometheus.GaugeVec
	//QPStats Requester DCQCN
	qpSqQcnCurrByteCounter       prometheus.GaugeVec
	qpSqQcnNumByteCounterExpired prometheus.GaugeVec
	qpSqQcnNumTimerExpired       prometheus.GaugeVec
	qpSqQcnNumAlphaTimerExpired  prometheus.GaugeVec
	qpSqQcnNumCNPrcvd            prometheus.GaugeVec
	qpSqQcnNumCNPprocessed       prometheus.GaugeVec
	//QPStats Responder TX
	qpRqRspTxNumPackets          prometheus.GaugeVec
	qpRqRspTxRnrError            prometheus.GaugeVec
	qpRqRspTxNumSequenceError    prometheus.GaugeVec
	qpRqRspTxRPByteThresholdHits prometheus.GaugeVec
	qpRqRspTxRPMaxRateHits       prometheus.GaugeVec
	//QPStats Responder RX
	qpRqRspRxNumPackets          prometheus.GaugeVec
	qpRqRspRxNumSendMsgsRke      prometheus.GaugeVec
	qpRqRspRxNumPacketsEcnMarked prometheus.GaugeVec
	qpRqRspRxNumCNPsReceived     prometheus.GaugeVec
	qpRqRspRxMaxRecircDrop       prometheus.GaugeVec
	qpRqRspRxNumMemWindowInvalid prometheus.GaugeVec
	qpRqRspRxNumDuplWriteSendOpc prometheus.GaugeVec
	qpRqRspRxNumDupReadBacktrack prometheus.GaugeVec
	qpRqRspRxNumDupReadDrop      prometheus.GaugeVec
	//QPStats Responder DCQCN
	qpRqQcnCurrByteCounter       prometheus.GaugeVec
	qpRqQcnNumByteCounterExpired prometheus.GaugeVec
	qpRqQcnNumTimerExpired       prometheus.GaugeVec
	qpRqQcnNumAlphaTimerExpired  prometheus.GaugeVec
	qpRqQcnNumCNPrcvd            prometheus.GaugeVec
	qpRqQcnNumCNPprocessed       prometheus.GaugeVec
}

func (na *NICAgentClient) ResetMetrics() error {
	// reset all label based fields
	na.m.nicMaxSpeed.Reset()
	na.m.nicPortStatsFramesRxBadFcs.Reset()
	na.m.nicPortStatsFramesRxBadAll.Reset()
	na.m.nicPortStatsFramesRxPause.Reset()
	na.m.nicPortStatsFramesRxBadLength.Reset()
	na.m.nicPortStatsFramesRxUndersized.Reset()
	na.m.nicPortStatsFramesRxOversized.Reset()
	na.m.nicPortStatsFramesRxFragments.Reset()
	na.m.nicPortStatsFramesRxJabber.Reset()
	na.m.nicPortStatsFramesRxPripause.Reset()
	na.m.nicPortStatsFramesRxStompedCrc.Reset()
	na.m.nicPortStatsFramesRxTooLong.Reset()
	na.m.nicPortStatsFramesRxDropped.Reset()
	na.m.nicPortStatsFramesTxBad.Reset()
	na.m.nicPortStatsFramesTxPause.Reset()
	na.m.nicPortStatsFramesTxPripause.Reset()
	na.m.nicPortStatsFramesTxLessThan64b.Reset()
	na.m.nicPortStatsFramesTxTruncated.Reset()
	na.m.nicPortStatsRsfecCorrectableWord.Reset()
	na.m.nicPortStatsRsfecChSymbolErrCnt.Reset()
	na.m.rdmaTxUcastPkts.Reset()
	na.m.rdmaTxCnpPkts.Reset()
	na.m.rdmaRxUcastPkts.Reset()
	na.m.rdmaRxCnpPkts.Reset()
	na.m.rdmaRxEcnPkts.Reset()
	na.m.rdmaReqRxPktSeqErr.Reset()
	na.m.rdmaReqRxRnrRetryErr.Reset()
	na.m.rdmaReqRxRmtAccErr.Reset()
	na.m.rdmaReqRxRmtReqErr.Reset()
	na.m.rdmaReqRxOperErr.Reset()
	na.m.rdmaReqRxImplNakSeqErr.Reset()
	na.m.rdmaReqRxCqeErr.Reset()
	na.m.rdmaReqRxCqeFlush.Reset()
	na.m.rdmaReqRxDupResp.Reset()
	na.m.rdmaReqRxInvalidPkts.Reset()

	na.m.rdmaReqTxLocErr.Reset()
	na.m.rdmaReqTxLocOperErr.Reset()
	na.m.rdmaReqTxMemMgmtErr.Reset()
	na.m.rdmaReqTxRetryExcdErr.Reset()
	na.m.rdmaReqTxLocSglInvErr.Reset()

	na.m.rdmaRespRxDupRequest.Reset()
	na.m.rdmaRespRxOutofBuf.Reset()
	na.m.rdmaRespRxOutoufSeq.Reset()
	na.m.rdmaRespRxCqeErr.Reset()
	na.m.rdmaRespRxCqeFlush.Reset()
	na.m.rdmaRespRxLocLenErr.Reset()
	na.m.rdmaRespRxInvalidRequest.Reset()
	na.m.rdmaRespRxLocOperErr.Reset()
	na.m.rdmaRespRxOutofAtomic.Reset()

	na.m.rdmaRespTxPktSeqErr.Reset()
	na.m.rdmaRespTxRmtInvalReqErr.Reset()
	na.m.rdmaRespTxRmtAccErr.Reset()
	na.m.rdmaRespTxRmtOperErr.Reset()
	na.m.rdmaRespTxRnrRetryErr.Reset()
	na.m.rdmaRespTxLocSglInvErr.Reset()

	na.m.rdmaRespRxS0TableErr.Reset()

	na.m.nicLifStatsRxUnicastPackets.Reset()
	na.m.nicLifStatsRxUnicastDropPackets.Reset()
	na.m.nicLifStatsRxMulticastDropPackets.Reset()
	na.m.nicLifStatsRxBroadcastDropPackets.Reset()
	na.m.nicLifStatsRxDMAErrors.Reset()
	na.m.nicLifStatsTxUnicastPackets.Reset()
	na.m.nicLifStatsTxUnicastDropPackets.Reset()
	na.m.nicLifStatsTxMulticastDropPackets.Reset()
	na.m.nicLifStatsTxBroadcastDropPackets.Reset()
	na.m.nicLifStatsTxDMAErrors.Reset()

	na.m.qpSqReqTxNumPackets.Reset()
	na.m.qpSqReqTxNumSendMsgsRke.Reset()
	na.m.qpSqReqTxNumLocalAckTimeouts.Reset()
	na.m.qpSqReqTxRnrTimeout.Reset()
	na.m.qpSqReqTxTimesSQdrained.Reset()
	na.m.qpSqReqTxNumCNPsent.Reset()

	na.m.qpSqReqRxNumPackets.Reset()
	na.m.qpSqReqRxNumPacketsEcnMarked.Reset()

	na.m.qpSqQcnCurrByteCounter.Reset()
	na.m.qpSqQcnNumByteCounterExpired.Reset()
	na.m.qpSqQcnNumTimerExpired.Reset()
	na.m.qpSqQcnNumAlphaTimerExpired.Reset()
	na.m.qpSqQcnNumCNPrcvd.Reset()
	na.m.qpSqQcnNumCNPprocessed.Reset()

	na.m.qpRqRspTxNumPackets.Reset()
	na.m.qpRqRspTxRnrError.Reset()
	na.m.qpRqRspTxNumSequenceError.Reset()
	na.m.qpRqRspTxRPByteThresholdHits.Reset()
	na.m.qpRqRspTxRPMaxRateHits.Reset()

	na.m.qpRqRspRxNumPackets.Reset()
	na.m.qpRqRspRxNumSendMsgsRke.Reset()
	na.m.qpRqRspRxNumPacketsEcnMarked.Reset()
	na.m.qpRqRspRxNumCNPsReceived.Reset()
	na.m.qpRqRspRxMaxRecircDrop.Reset()
	na.m.qpRqRspRxNumMemWindowInvalid.Reset()
	na.m.qpRqRspRxNumDuplWriteSendOpc.Reset()
	na.m.qpRqRspRxNumDupReadBacktrack.Reset()
	na.m.qpRqRspRxNumDupReadDrop.Reset()

	na.m.qpRqQcnCurrByteCounter.Reset()
	na.m.qpRqQcnNumByteCounterExpired.Reset()
	na.m.qpRqQcnNumTimerExpired.Reset()
	na.m.qpRqQcnNumAlphaTimerExpired.Reset()
	na.m.qpRqQcnNumCNPrcvd.Reset()
	na.m.qpRqQcnNumCNPprocessed.Reset()
	return nil
}

func (na *NICAgentClient) GetExporterNonNICLabels() []string {
	labelList := []string{
		strings.ToLower(exportermetrics.NICMetricLabel_NIC_HOSTNAME.String()),
	}
	// Add custom labels
	for label := range customLabelMap {
		labelList = append(labelList, strings.ToLower(label))
	}
	return labelList
}

func (na *NICAgentClient) GetExportLabels() []string { //TODO .. move to exporter/utils
	labelList := []string{}
	for key, enabled := range exportLabels {
		if !enabled {
			continue
		}
		labelList = append(labelList, strings.ToLower(key))
	}

	// process extra pod labels
	for key := range extraPodLabelsMap {
		exists := false
		for _, label := range labelList {
			if key == label {
				exists = true
				break
			}
		}
		if !exists {
			labelList = append(labelList, key)
		}
	}

	// process custom labels
	for key := range customLabelMap {
		exists := false
		for _, label := range labelList {
			if key == label {
				exists = true
				break
			}
		}

		// Add only unique labels to export labels
		if !exists {
			labelList = append(labelList, strings.ToLower(key))
		}
	}

	logger.Log.Printf("Get_export-labels returns %v", labelList)
	return labelList
}

func (na *NICAgentClient) initLabelConfigs(config *exportermetrics.NICMetricConfig) {
	// list of mandatory labels
	exportLabels = make(map[string]bool)
	for _, name := range exportermetrics.NICMetricLabel_name {
		exportLabels[name] = false
	}
	// only mandatory labels are set for default
	for _, name := range mandatoryLables {
		exportLabels[name] = true
	}

	k8sLabels := scheduler.GetExportLabels(scheduler.Kubernetes)

	if config != nil {
		for _, name := range config.GetLabels() {
			name = strings.ToUpper(name)
			if _, ok := exportLabels[name]; ok {
				// export labels must have atleast one label exported by
				// kubernets client, otherwise don't enable the label
				if _, ok := k8sLabels[name]; ok && !na.isKubernetes {
					continue
				}

				logger.Log.Printf("label %v enabled", name)
				exportLabels[name] = true
			}
		}
	}
	logger.Log.Printf("export-labels updated to %v", exportLabels)
}

func (na *NICAgentClient) initCustomLabels(config *exportermetrics.NICMetricConfig) {
	customLabelMap = make(map[string]string)
	if config != nil && config.GetCustomLabels() != nil {
		cl := config.GetCustomLabels()
		labelCount := 0

		for l, value := range cl {
			if labelCount >= globals.MaxSupportedCustomLabels {
				logger.Log.Printf("Max custom labels supported: %v, ignoring extra labels.", globals.MaxSupportedCustomLabels)
				break
			}
			label := strings.ToLower(l)

			// Check if custom label is a mandatory label, ignore if true
			found := false
			for _, mlabel := range mandatoryLables {
				if strings.ToLower(mlabel) == label {
					logger.Log.Printf("Detected mandatory label %s in custom label, ignoring...", mlabel)
					found = true
					break
				}
			}
			if found {
				continue
			}

			// Store all custom labels
			customLabelMap[label] = value
			labelCount++
		}
	}
	logger.Log.Printf("custom labels being exported: %v", customLabelMap)
}

func (na *NICAgentClient) initFieldConfig(config *exportermetrics.NICMetricConfig) {
	exportFieldMap = make(map[string]bool)
	// setup metric fields in map to be monitored
	// init the map with all supported strings from enum
	enable_default := true
	if config != nil && len(config.GetFields()) != 0 {
		enable_default = false
	}
	for _, name := range exportermetrics.NICMetricField_name {
		exportFieldMap[name] = enable_default
	}
	if config == nil || len(config.GetFields()) == 0 {
		return
	}
	for _, fieldName := range config.GetFields() {
		fieldName = strings.ToUpper(fieldName)
		if _, ok := exportFieldMap[fieldName]; ok {
			exportFieldMap[fieldName] = true
		}
	}
	// print disabled short list
	for k, v := range exportFieldMap {
		if !v {
			logger.Log.Printf("%v field is disabled", k)
		}
	}
}

func (na *NICAgentClient) initFieldMetricsMap() {
	// must follow index mapping to fields.proto (NICMetricField)
	fieldMetricsMap = []prometheus.Collector{
		na.m.nicNodesTotal,
		na.m.nicMaxSpeed,
		na.m.nicPortStatsFramesRxBadFcs,
		na.m.nicPortStatsFramesRxBadAll,
		na.m.nicPortStatsFramesRxPause,
		na.m.nicPortStatsFramesRxBadLength,
		na.m.nicPortStatsFramesRxUndersized,
		na.m.nicPortStatsFramesRxOversized,
		na.m.nicPortStatsFramesRxFragments,
		na.m.nicPortStatsFramesRxJabber,
		na.m.nicPortStatsFramesRxPripause,
		na.m.nicPortStatsFramesRxStompedCrc,
		na.m.nicPortStatsFramesRxTooLong,
		na.m.nicPortStatsFramesRxDropped,
		na.m.nicPortStatsFramesTxBad,
		na.m.nicPortStatsFramesTxPause,
		na.m.nicPortStatsFramesTxPripause,
		na.m.nicPortStatsFramesTxLessThan64b,
		na.m.nicPortStatsFramesTxTruncated,
		na.m.nicPortStatsRsfecCorrectableWord,
		na.m.nicPortStatsRsfecChSymbolErrCnt,
		na.m.rdmaTxUcastPkts,
		na.m.rdmaTxCnpPkts,
		na.m.rdmaRxUcastPkts,
		na.m.rdmaRxCnpPkts,
		na.m.rdmaRxEcnPkts,
		na.m.rdmaReqRxPktSeqErr,
		na.m.rdmaReqRxRnrRetryErr,
		na.m.rdmaReqRxRmtAccErr,
		na.m.rdmaReqRxRmtReqErr,
		na.m.rdmaReqRxOperErr,
		na.m.rdmaReqRxImplNakSeqErr,
		na.m.rdmaReqRxCqeErr,
		na.m.rdmaReqRxCqeFlush,
		na.m.rdmaReqRxDupResp,
		na.m.rdmaReqRxInvalidPkts,

		na.m.rdmaReqTxLocErr,
		na.m.rdmaReqTxLocOperErr,
		na.m.rdmaReqTxMemMgmtErr,
		na.m.rdmaReqTxRetryExcdErr,
		na.m.rdmaReqTxLocSglInvErr,

		na.m.rdmaRespRxDupRequest,
		na.m.rdmaRespRxOutofBuf,
		na.m.rdmaRespRxOutoufSeq,
		na.m.rdmaRespRxCqeErr,
		na.m.rdmaRespRxCqeFlush,
		na.m.rdmaRespRxLocLenErr,
		na.m.rdmaRespRxInvalidRequest,
		na.m.rdmaRespRxLocOperErr,
		na.m.rdmaRespRxOutofAtomic,

		na.m.rdmaRespTxPktSeqErr,
		na.m.rdmaRespTxRmtInvalReqErr,
		na.m.rdmaRespTxRmtAccErr,
		na.m.rdmaRespTxRmtOperErr,
		na.m.rdmaRespTxRnrRetryErr,
		na.m.rdmaRespTxLocSglInvErr,

		na.m.rdmaRespRxS0TableErr,

		na.m.nicLifStatsRxUnicastPackets,
		na.m.nicLifStatsRxUnicastDropPackets,
		na.m.nicLifStatsRxMulticastDropPackets,
		na.m.nicLifStatsRxBroadcastDropPackets,
		na.m.nicLifStatsRxDMAErrors,
		na.m.nicLifStatsTxUnicastPackets,
		na.m.nicLifStatsTxUnicastDropPackets,
		na.m.nicLifStatsTxMulticastDropPackets,
		na.m.nicLifStatsTxBroadcastDropPackets,
		na.m.nicLifStatsTxDMAErrors,

		na.m.qpSqReqTxNumPackets,
		na.m.qpSqReqTxNumSendMsgsRke,
		na.m.qpSqReqTxNumLocalAckTimeouts,
		na.m.qpSqReqTxRnrTimeout,
		na.m.qpSqReqTxTimesSQdrained,
		na.m.qpSqReqTxNumCNPsent,

		na.m.qpSqReqRxNumPackets,
		na.m.qpSqReqRxNumPacketsEcnMarked,

		na.m.qpSqQcnCurrByteCounter,
		na.m.qpSqQcnNumByteCounterExpired,
		na.m.qpSqQcnNumTimerExpired,
		na.m.qpSqQcnNumAlphaTimerExpired,
		na.m.qpSqQcnNumCNPrcvd,
		na.m.qpSqQcnNumCNPprocessed,

		na.m.qpRqRspTxNumPackets,
		na.m.qpRqRspTxRnrError,
		na.m.qpRqRspTxNumSequenceError,
		na.m.qpRqRspTxRPByteThresholdHits,
		na.m.qpRqRspTxRPMaxRateHits,

		na.m.qpRqRspRxNumPackets,
		na.m.qpRqRspRxNumSendMsgsRke,
		na.m.qpRqRspRxNumPacketsEcnMarked,
		na.m.qpRqRspRxNumCNPsReceived,
		na.m.qpRqRspRxMaxRecircDrop,
		na.m.qpRqRspRxNumMemWindowInvalid,
		na.m.qpRqRspRxNumDuplWriteSendOpc,
		na.m.qpRqRspRxNumDupReadBacktrack,
		na.m.qpRqRspRxNumDupReadDrop,

		na.m.qpRqQcnCurrByteCounter,
		na.m.qpRqQcnNumByteCounterExpired,
		na.m.qpRqQcnNumTimerExpired,
		na.m.qpRqQcnNumAlphaTimerExpired,
		na.m.qpRqQcnNumCNPrcvd,
		na.m.qpRqQcnNumCNPprocessed,
	}
}

func (na *NICAgentClient) initPrometheusMetrics() {
	nonNICLabels := na.GetExporterNonNICLabels()
	labels := na.GetExportLabels()
	na.m = &metrics{
		nicNodesTotal: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_TOTAL.String()),
			Help: "Number of NICs in the node",
		}, nonNICLabels),

		nicMaxSpeed: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_MAX_SPEED.String()),
			Help: "Maximum NIC speed in Gbps",
		}, labels),

		nicPortStatsFramesRxBadFcs: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_PORT_STATS_FRAMES_RX_BAD_FCS.String()),
			Help: "Bad frames received due to a Frame Check Sequence (FCS) error on a network port",
		}, append([]string{LabelPortName, LabelPortID}, labels...)),

		nicPortStatsFramesRxBadAll: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_PORT_STATS_FRAMES_RX_BAD_ALL.String()),
			Help: "Total number of frames received on a network port that are bad",
		}, append([]string{LabelPortName, LabelPortID}, labels...)),

		nicPortStatsFramesRxPause: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_PORT_STATS_FRAMES_RX_PAUSE.String()),
			Help: "Total number of pause frames received on a network port",
		}, append([]string{LabelPortName, LabelPortID}, labels...)),

		nicPortStatsFramesRxBadLength: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_PORT_STATS_FRAMES_RX_BAD_LENGTH.String()),
			Help: "Total number of frames received that have an incorrect or invalid length",
		}, append([]string{LabelPortName, LabelPortID}, labels...)),

		nicPortStatsFramesRxUndersized: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_PORT_STATS_FRAMES_RX_UNDERSIZED.String()),
			Help: "Total number of frames received that are smaller than the minimum frame size allowed by the network protocol",
		}, append([]string{LabelPortName, LabelPortID}, labels...)),

		nicPortStatsFramesRxOversized: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_PORT_STATS_FRAMES_RX_OVERSIZED.String()),
			Help: " Total number of frames received that exceed the maximum allowed size for the network protocol",
		}, append([]string{LabelPortName, LabelPortID}, labels...)),

		nicPortStatsFramesRxFragments: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_PORT_STATS_FRAMES_RX_FRAGMENTS.String()),
			Help: "Total number of frames received that are fragments of larger packets",
		}, append([]string{LabelPortName, LabelPortID}, labels...)),

		nicPortStatsFramesRxJabber: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_PORT_STATS_FRAMES_RX_JABBER.String()),
			Help: "Total number of frames received that are considered jabber frames",
		}, append([]string{LabelPortName, LabelPortID}, labels...)),

		nicPortStatsFramesRxPripause: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_PORT_STATS_FRAMES_RX_PRIPAUSE.String()),
			Help: "Total number of priority pause frames received",
		}, append([]string{LabelPortName, LabelPortID}, labels...)),

		nicPortStatsFramesRxStompedCrc: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_PORT_STATS_FRAMES_RX_STOMPED_CRC.String()),
			Help: "Total number of frames received that had a valid CRC (Cyclic Redundancy Check) but were stomped",
		}, append([]string{LabelPortName, LabelPortID}, labels...)),

		nicPortStatsFramesRxTooLong: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_PORT_STATS_FRAMES_RX_TOO_LONG.String()),
			Help: "Total number of frames received that exceed the maximum allowable size for frames on the network",
		}, append([]string{LabelPortName, LabelPortID}, labels...)),

		nicPortStatsFramesRxDropped: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_PORT_STATS_FRAMES_RX_DROPPED.String()),
			Help: "Total number of frames that were received but dropped due to various reasons such as buffer overflows or hardware limitations",
		}, append([]string{LabelPortName, LabelPortID}, labels...)),

		nicPortStatsFramesTxBad: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_PORT_STATS_FRAMES_TX_BAD.String()),
			Help: "Total number of transmitted frames that are considered bad",
		}, append([]string{LabelPortName, LabelPortID}, labels...)),

		nicPortStatsFramesTxPause: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_PORT_STATS_FRAMES_TX_PAUSE.String()),
			Help: "Total number of pause frames transmitted",
		}, append([]string{LabelPortName, LabelPortID}, labels...)),

		nicPortStatsFramesTxPripause: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_PORT_STATS_FRAMES_TX_PRIPAUSE.String()),
			Help: "Total number of priority pause frames transmitted",
		}, append([]string{LabelPortName, LabelPortID}, labels...)),

		nicPortStatsFramesTxLessThan64b: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_PORT_STATS_FRAMES_TX_LESS_THAN_64B.String()),
			Help: "Total number of frames transmitted that are smaller than the minimum frame size i.e 64 bytes",
		}, append([]string{LabelPortName, LabelPortID}, labels...)),

		nicPortStatsFramesTxTruncated: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_PORT_STATS_FRAMES_TX_TRUNCATED.String()),
			Help: "Total number of frames that were transmitted but truncated",
		}, append([]string{LabelPortName, LabelPortID}, labels...)),

		nicPortStatsRsfecCorrectableWord: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_PORT_STATS_RSFEC_CORRECTABLE_WORD.String()),
			Help: "Total number of RS-FEC (Reed-Solomon Forward Error Correction) correctable words received or transmitted",
		}, append([]string{LabelPortName, LabelPortID}, labels...)),

		nicPortStatsRsfecChSymbolErrCnt: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_PORT_STATS_RSFEC_CH_SYMBOL_ERR_CNT.String()),
			Help: "Total count of channel symbol errors detected by the RS-FEC (Reed-Solomon Forward Error Correction) mechanism.",
		}, append([]string{LabelPortName, LabelPortID}, labels...)),

		rdmaTxUcastPkts: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_TX_UCAST_PKTS.String()),
			Help: "Tx RDMA Unicast Packets",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaTxCnpPkts: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_TX_CNP_PKTS.String()),
			Help: "Tx RDMA Congestion Notification Packets",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaRxUcastPkts: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_RX_UCAST_PKTS.String()),
			Help: "Rx RDMA Ucast Pkts ",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaRxCnpPkts: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_RX_CNP_PKTS.String()),
			Help: "Rx RDMA Congestion Notification Packets",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaRxEcnPkts: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_RX_ECN_PKTS.String()),
			Help: "Rx RDMA Explicit Congestion Notification Packets",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaReqRxPktSeqErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_REQ_RX_PKT_SEQ_ERR.String()),
			Help: "Request Rx packet sequence errors",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaReqRxRnrRetryErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_REQ_RX_RNR_RETRY_ERR.String()),
			Help: "Request Rx receiver not ready retry errors",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaReqRxRmtAccErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_REQ_RX_RMT_ACC_ERR.String()),
			Help: "Request Rx remote access errors",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaReqRxRmtReqErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_REQ_RX_RMT_REQ_ERR.String()),
			Help: "Request Rx remote request errors",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaReqRxOperErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_REQ_RX_OPER_ERR.String()),
			Help: "Request Rx remote oper errors",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaReqRxImplNakSeqErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_REQ_RX_IMPL_NAK_SEQ_ERR.String()),
			Help: "Request Rx implicit negative acknowledgment errors",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaReqRxCqeErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_REQ_RX_CQE_ERR.String()),
			Help: "Request Rx completion queue errors",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaReqRxCqeFlush: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_REQ_RX_CQE_FLUSH.String()),
			Help: "Request Rx completion queue flush count",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaReqRxDupResp: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_REQ_RX_DUP_RESP.String()),
			Help: "Request Rx duplicate response errors",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaReqRxInvalidPkts: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_REQ_RX_INVALID_PKTS.String()),
			Help: "Request Rx invalid pkts ",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaReqTxLocErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_REQ_TX_LOC_ERR.String()),
			Help: "Request Tx local errors",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaReqTxLocOperErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_REQ_TX_LOC_OPER_ERR.String()),
			Help: "Request Tx local operation errors",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaReqTxMemMgmtErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_REQ_TX_MEM_MGMT_ERR.String()),
			Help: "Request Tx memory management errors ",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaReqTxRetryExcdErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_REQ_TX_RETRY_EXCD_ERR.String()),
			Help: "Request Tx Retry exceeded errors ",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaReqTxLocSglInvErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_REQ_TX_LOC_SGL_INV_ERR.String()),
			Help: "Request Tx local signal inversion errors ",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaRespRxDupRequest: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_RESP_RX_DUP_REQUEST.String()),
			Help: "Response Rx duplicate request count",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaRespRxOutofBuf: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_RESP_RX_OUTOF_BUF.String()),
			Help: "Response Rx out of buffer count",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaRespRxOutoufSeq: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_RESP_RX_OUTOUF_SEQ.String()),
			Help: "Response Rx out of sequence count",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaRespRxCqeErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_RESP_RX_CQE_ERR.String()),
			Help: "Response Rx completion queue errors",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaRespRxCqeFlush: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_RESP_RX_CQE_FLUSH.String()),
			Help: "Response Rx completion queue flush",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaRespRxLocLenErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_RESP_RX_LOC_LEN_ERR.String()),
			Help: "Response Rx local length errors",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaRespRxInvalidRequest: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_RESP_RX_INVALID_REQUEST.String()),
			Help: "Response Rx invalid requests count",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaRespRxLocOperErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_RESP_RX_LOC_OPER_ERR.String()),
			Help: "Response Rx local operation errors",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaRespRxOutofAtomic: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_RESP_RX_OUTOF_ATOMIC.String()),
			Help: "Response Rx without atomic guarantee count",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaRespTxPktSeqErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_RESP_TX_PKT_SEQ_ERR.String()),
			Help: "Response Tx packet sequence error count",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaRespTxRmtInvalReqErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_RESP_TX_RMT_INVAL_REQ_ERR.String()),
			Help: "Response Tx remote invalid request count",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaRespTxRmtAccErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_RESP_TX_RMT_ACC_ERR.String()),
			Help: "Response Tx remote access error count",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaRespTxRmtOperErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_RESP_TX_RMT_OPER_ERR.String()),
			Help: "Response Tx remote operation error count",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaRespTxRnrRetryErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_RESP_TX_RNR_RETRY_ERR.String()),
			Help: "Response Tx retry not required error count",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaRespTxLocSglInvErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_RESP_TX_LOC_SGL_INV_ERR.String()),
			Help: "Response Tx local signal inversion error count",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		rdmaRespRxS0TableErr: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_RDMA_RESP_RX_S0_TABLE_ERR.String()),
			Help: "Response rx S0 Table error count",
		}, append(append([]string{LabelRdmaIfName, LabelRdmaIfPcieAddr}, labels...), workloadLabels...)),

		/* Lif stats */
		nicLifStatsRxUnicastPackets: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_LIF_STATS_RX_UNICAST_PACKETS.String()),
			Help: "Total number of unicast packets received by the NIC",
		}, append(append([]string{LabelPortName, LabelLifName}, labels...), workloadLabels...)),

		nicLifStatsRxUnicastDropPackets: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_LIF_STATS_RX_UNICAST_DROP_PACKETS.String()),
			Help: "Number of unicast packets that were dropped during reception",
		}, append(append([]string{LabelPortName, LabelLifName}, labels...), workloadLabels...)),

		nicLifStatsRxMulticastDropPackets: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_LIF_STATS_RX_MULTICAST_DROP_PACKETS.String()),
			Help: "Number of multicast packets that were dropped during reception",
		}, append(append([]string{LabelPortName, LabelLifName}, labels...), workloadLabels...)),

		nicLifStatsRxBroadcastDropPackets: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_LIF_STATS_RX_BROADCAST_DROP_PACKETS.String()),
			Help: "Number of broadcast packets that were dropped during reception",
		}, append(append([]string{LabelPortName, LabelLifName}, labels...), workloadLabels...)),

		nicLifStatsRxDMAErrors: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_LIF_STATS_RX_DMA_ERRORS.String()),
			Help: "Number of errors encountered while performing Direct Memory Access (DMA) during packet reception",
		}, append(append([]string{LabelPortName, LabelLifName}, labels...), workloadLabels...)),

		nicLifStatsTxUnicastPackets: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_LIF_STATS_TX_UNICAST_PACKETS.String()),
			Help: "Total number of unicast packets transmitted by the NIC",
		}, append(append([]string{LabelPortName, LabelLifName}, labels...), workloadLabels...)),

		nicLifStatsTxUnicastDropPackets: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_LIF_STATS_TX_UNICAST_DROP_PACKETS.String()),
			Help: "Number of unicast packets that were dropped during transmission",
		}, append(append([]string{LabelPortName, LabelLifName}, labels...), workloadLabels...)),

		nicLifStatsTxMulticastDropPackets: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_LIF_STATS_TX_MULTICAST_DROP_PACKETS.String()),
			Help: "Number of multicast packets that were dropped during transmission",
		}, append(append([]string{LabelPortName, LabelLifName}, labels...), workloadLabels...)),

		nicLifStatsTxBroadcastDropPackets: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_LIF_STATS_TX_BROADCAST_DROP_PACKETS.String()),
			Help: "Number of broadcast packets that were dropped during transmission",
		}, append(append([]string{LabelPortName, LabelLifName}, labels...), workloadLabels...)),

		nicLifStatsTxDMAErrors: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_NIC_LIF_STATS_TX_DMA_ERRORS.String()),
			Help: "Number of errors encountered while performing Direct Memory Access (DMA) during packet transmission",
		}, append(append([]string{LabelPortName, LabelLifName}, labels...), workloadLabels...)),

		/* QP  stats */
		qpSqReqTxNumPackets: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_SQ_REQ_TX_NUM_PACKET.String()),
			Help: "SendQueue Requester Tx packets ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpSqReqTxNumSendMsgsRke: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_SQ_REQ_TX_NUM_SEND_MSGS_WITH_RKE.String()),
			Help: "SendQueue Requester Tx num send msgs with invalid remote key error ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpSqReqTxNumLocalAckTimeouts: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_SQ_REQ_TX_NUM_LOCAL_ACK_TIMEOUTS.String()),
			Help: "SendQueue Requester Tx local ACK timeouts ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpSqReqTxRnrTimeout: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_SQ_REQ_TX_RNR_TIMEOUT.String()),
			Help: "SendQueue Requester Tx receiver not ready timeouts ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpSqReqTxTimesSQdrained: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_SQ_REQ_TX_TIMES_SQ_DRAINED.String()),
			Help: "SendQueue Requester Tx times Send queue is drained ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpSqReqTxNumCNPsent: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_SQ_REQ_TX_NUM_CNP_SENT.String()),
			Help: "SendQueue Requester Tx number of Congestion notification packets sents ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),

		qpSqReqRxNumPackets: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_SQ_REQ_RX_NUM_PACKET.String()),
			Help: "SendQueue Requester Rx packets ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpSqReqRxNumPacketsEcnMarked: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_SQ_REQ_RX_NUM_PKTS_WITH_ECN_MARKING.String()),
			Help: "SendQueue Requester Rx packets with explicit congestion notification marking ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),

		qpSqQcnCurrByteCounter: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_SQ_QCN_CURR_BYTE_COUNTER.String()),
			Help: "SendQueue DCQCN Current Byte Counter ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpSqQcnNumByteCounterExpired: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_SQ_QCN_NUM_BYTE_COUNTER_EXPIRED.String()),
			Help: "SendQueue DCQCN number of byte counter expired",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpSqQcnNumTimerExpired: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_SQ_QCN_NUM_TIMER_EXPIRED.String()),
			Help: "SendQueue DCQCN number of timer expired",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpSqQcnNumAlphaTimerExpired: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_SQ_QCN_NUM_ALPHA_TIMER_EXPIRED.String()),
			Help: "SendQueue DCQCN number of alpha timer expired",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpSqQcnNumCNPrcvd: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_SQ_QCN_NUM_CNP_RCVD.String()),
			Help: "SendQueue DCQCN number of Congestion notification packets received",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpSqQcnNumCNPprocessed: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_SQ_QCN_NUM_CNP_PROCESSED.String()),
			Help: "SendQueue DCQCN number of Congestion notification packets processed",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),

		qpRqRspTxNumPackets: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_RSP_TX_NUM_PACKET.String()),
			Help: "RecvQueue Responder Tx number of packets ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpRqRspTxRnrError: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_RSP_TX_RNR_ERROR.String()),
			Help: "RecvQueue Responder Tx receiver nor ready errors ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpRqRspTxNumSequenceError: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_RSP_TX_NUM_SEQUENCE_ERROR.String()),
			Help: "RecvQueue Responder Tx number of sequence errors ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpRqRspTxRPByteThresholdHits: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_RSP_TX_NUM_RP_BYTE_THRES_HIT.String()),
			Help: "RecvQueue Responder Tx number of RP byte threhshold hit ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpRqRspTxRPMaxRateHits: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_RSP_TX_NUM_RP_MAX_RATE_HIT.String()),
			Help: "RecvQueue Responder Tx number of RP max rate hit ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),

		qpRqRspRxNumPackets: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_RSP_RX_NUM_PACKET.String()),
			Help: "RecvQueue Responder Rx number of packets",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpRqRspRxNumSendMsgsRke: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_RSP_RX_NUM_SEND_MSGS_WITH_RKE.String()),
			Help: "RecvQueue Responder Rx number of send msgs with invalid remote key error ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpRqRspRxNumPacketsEcnMarked: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_RSP_RX_NUM_PKTS_WITH_ECN_MARKING.String()),
			Help: "RecvQueue Responder Rx number of pkts with ECN marking ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpRqRspRxNumCNPsReceived: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_RSP_RX_NUM_CNPS_RECEIVED.String()),
			Help: "RecvQueue Responder Rx number of CNP pkts ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpRqRspRxMaxRecircDrop: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_RSP_RX_MAX_RECIRC_EXCEEDED_DROP.String()),
			Help: "RecvQueue Responder Rx max recirculation execeeded packet drop ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpRqRspRxNumMemWindowInvalid: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_RSP_RX_NUM_MEM_WINDOW_INVALID.String()),
			Help: "RecvQueue Responder Rx number of memory window invalidate msg ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpRqRspRxNumDuplWriteSendOpc: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_RSP_RX_NUM_DUPL_WITH_WR_SEND_OPC.String()),
			Help: "RecvQueue Responder Rx number of duplicate pkts with write send opcode ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpRqRspRxNumDupReadBacktrack: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_RSP_RX_NUM_DUPL_READ_BACKTRACK.String()),
			Help: "RecvQueue Responder Rx number of duplicate read atomic backtrack packet ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpRqRspRxNumDupReadDrop: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_RSP_RX_NUM_DUPL_READ_ATOMIC_DROP.String()),
			Help: "RecvQueue Responder Rx number of duplicate read atomic backtrack packet ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),

		qpRqQcnCurrByteCounter: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_QCN_CURR_BYTE_COUNTER.String()),
			Help: "RecvQueue DCQCN Current Byte Counter ",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpRqQcnNumByteCounterExpired: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_QCN_NUM_BYTE_COUNTER_EXPIRED.String()),
			Help: "RecvQueue DCQCN number of byte counter expired",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpRqQcnNumTimerExpired: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_QCN_NUM_TIMER_EXPIRED.String()),
			Help: "RecvQueue DCQCN number of timer expired",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpRqQcnNumAlphaTimerExpired: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_QCN_NUM_ALPHA_TIMER_EXPIRED.String()),
			Help: "RecvQueue DCQCN number of alpha timer expired",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpRqQcnNumCNPrcvd: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_QCN_NUM_CNP_RCVD.String()),
			Help: "RecvQueue DCQCN number of Congestion notification packets received",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
		qpRqQcnNumCNPprocessed: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: strings.ToLower(exportermetrics.NICMetricField_QP_RQ_QCN_NUM_CNP_PROCESSED.String()),
			Help: "RecvQueue DCQCN number of Congestion notification packets processed",
		}, append(append([]string{LabelLifName, LabelQPID}, labels...), workloadLabels...)),
	}
	na.initFieldMetricsMap()
}

func (na *NICAgentClient) initFieldRegistration() error {
	for field, enabled := range exportFieldMap {
		if !enabled {
			continue
		}
		fieldIndex, ok := exportermetrics.NICMetricField_value[field]
		if !ok {
			logger.Log.Printf("Invalid field %v, ignored", field)
			continue
		}
		na.mh.GetRegistry().MustRegister(fieldMetricsMap[fieldIndex])
	}

	return nil
}

func (na *NICAgentClient) initPodExtraLabels(config *exportermetrics.NICMetricConfig) {
	// initialize pod labels maps
	k8PodLabelsMap = make(map[string]map[string]string)
	if config != nil {
		extraPodLabelsMap = utils.NormalizeExtraPodLabels(config.GetExtraPodLabels())
	}
	logger.Log.Printf("export-labels updated to %v", extraPodLabelsMap)
}

func (na *NICAgentClient) InitConfigs() error {
	filedConfigs := na.mh.GetNICMetricsConfig()

	na.initPodExtraLabels(filedConfigs)
	na.initCustomLabels(filedConfigs)
	na.initLabelConfigs(filedConfigs)
	na.initFieldConfig(filedConfigs)
	na.initPrometheusMetrics()
	return na.initFieldRegistration()
}

func (na *NICAgentClient) UpdateStaticMetrics() error {
	var err error
	k8PodLabelsMap, err = na.fetchPodLabelsForNode()
	if err != nil {
		logger.Log.Printf("Failed to fetch pod labels for node: %v", err)
		return err
	}
	labels := na.populateLabelsFromNIC("")
	na.m.nicNodesTotal.With(labels).Set(float64(len(na.nics)))
	return nil
}

func (na *NICAgentClient) UpdateMetricsStats() error {
	return na.getMetricsAll()
}

func (na *NICAgentClient) QueryMetrics() (interface{}, error) {
	return nil, nil
}

func (na *NICAgentClient) GetDeviceType() globals.DeviceType {
	return globals.NICDevice
}

func GetNICMandatoryLabels() []string {
	return mandatoryLables
}

func (na *NICAgentClient) GetNICCustomeLabels() map[string]string {
	return customLabelMap
}

func (na *NICAgentClient) populateLabelsFromNIC(UUID string) map[string]string {
	labels := make(map[string]string)

	nic, found := na.nics[UUID]
	if !found {
		logger.Log.Printf("could not find NIC: %s from the local cache", UUID)
	}

	for ckey, enabled := range exportLabels {
		if !enabled {
			continue
		}
		key := strings.ToLower(ckey)
		switch ckey {
		case exportermetrics.NICMetricLabel_NIC_UUID.String():
			if nic != nil {
				labels[key] = nic.UUID
			}
		case exportermetrics.NICMetricLabel_NIC_ID.String():
			if nic != nil {
				labels[key] = nic.Index
			}
		case exportermetrics.NICMetricLabel_NIC_SERIAL_NUMBER.String():
			if nic != nil {
				labels[key] = nic.SerialNumber
			}
		case exportermetrics.NICMetricLabel_NIC_HOSTNAME.String():
			labels[key] = na.staticHostLabels[exportermetrics.NICMetricLabel_NIC_HOSTNAME.String()]

		default:
			logger.Log.Printf("Invalid label is ignored %v", key)
		}
	}

	// these extra pod labels are overwritten when there is a workload associated with the NIC with the respective pod label values
	for prometheusLabel := range extraPodLabelsMap {
		if nic != nil {
			labels[strings.ToLower(prometheusLabel)] = ""
		}
	}

	// Add custom labels
	for label, value := range customLabelMap {
		labels[label] = value
	}
	return labels
}

func (na *NICAgentClient) getAssociatedWorkloadLabelsForPcieAddr(pcieAddr string, workloads map[string]scheduler.Workload) map[string]string {
	labels := map[string]string{
		strings.ToLower(exportermetrics.NICMetricLabel_NIC_POD.String()):       "",
		strings.ToLower(exportermetrics.NICMetricLabel_NIC_NAMESPACE.String()): "",
		strings.ToLower(exportermetrics.NICMetricLabel_NIC_CONTAINER.String()): "",
	}

	if wl, wlFound := workloads[pcieAddr]; wlFound {
		podInfo := wl.Info.(scheduler.PodResourceInfo)
		labels[strings.ToLower(exportermetrics.NICMetricLabel_NIC_POD.String())] = podInfo.Pod
		labels[strings.ToLower(exportermetrics.NICMetricLabel_NIC_NAMESPACE.String())] = podInfo.Namespace
		labels[strings.ToLower(exportermetrics.NICMetricLabel_NIC_CONTAINER.String())] = podInfo.Container

		// Add extra pod labels only if config has mapped any
		if len(extraPodLabelsMap) > 0 {
			podLabels := utils.GetPodLabels(podInfo, k8PodLabelsMap)
			// populate labels from extraPodLabelsMap; regarless of whether there is a workload or not
			for prometheusPodlabel, k8Podlabel := range extraPodLabelsMap {
				label := strings.ToLower(prometheusPodlabel)
				labels[label] = podLabels[k8Podlabel]
			}
		}
	}
	return labels
}

// getAssociatedWorkloadLabels returns the workload labels for a given NIC and LIF
func (na *NICAgentClient) getAssociatedWorkloadLabels(nicID string, lifID string, workloads map[string]scheduler.Workload) map[string]string {
	labels := map[string]string{
		strings.ToLower(exportermetrics.NICMetricLabel_NIC_POD.String()):       "",
		strings.ToLower(exportermetrics.NICMetricLabel_NIC_NAMESPACE.String()): "",
		strings.ToLower(exportermetrics.NICMetricLabel_NIC_CONTAINER.String()): "",
	}

	if _, nicFound := na.nics[nicID]; !nicFound {
		logger.Log.Printf("NIC %s not found in the local cache", nicID)
		return labels
	}
	if _, lifFound := na.nics[nicID].Lifs[lifID]; !lifFound {
		logger.Log.Printf("LIF %s not found in NIC %s", lifID, nicID)
		return labels
	}

	lif, found := na.nics[nicID].Lifs[lifID]
	if found && lif.PCIeAddress != "" {
		return na.getAssociatedWorkloadLabelsForPcieAddr(lif.PCIeAddress, workloads)
	}
	return labels
}

// getNICs fetches all the static data that we need related to NIC including port
func (na *NICAgentClient) getNICs() (map[string]*NIC, error) {
	type Response struct {
		NIC []struct {
			ID           string `json:"id"`
			ProductName  string `json:"product_name"`
			SerialNumber string `json:"serial_number"`
			EthBDF       string `json:"eth_bdf"`
			Port         []struct {
				Spec struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"spec"`
			} `json:"port"`
			Lif []struct {
				Spec struct {
					ID         string `json:"id"`
					MACAddress string `json:"mac_address"`
				} `json:"spec"`
				Status struct {
					Name string `json:"name"`
				} `json:"status"`
			} `json:"lif"`
		} `json:"nic"`
	}

	nics := map[string]*NIC{}

	nicResp, err := ExecWithContext("nicctl show card -j")
	if err != nil {
		logger.Log.Printf("failed to get nic data, err: %+v", err)
		return nics, err
	}
	var resp Response
	err = json.Unmarshal(nicResp, &resp)
	if err != nil {
		logger.Log.Printf("error unmarshalling nic data: %v", err)
		return nics, err
	}

	// fetch port details for each NIC
	for index, nic := range resp.NIC {
		nics[nic.ID] = &NIC{
			Index:        fmt.Sprintf("%v", index),
			UUID:         nic.ID,
			ProductName:  nic.ProductName,
			SerialNumber: nic.SerialNumber,
			EthBDF:       nic.EthBDF,
		}

		cmd := fmt.Sprintf("nicctl show port --card %s -j", nic.ID)
		portResp, err := ExecWithContext(cmd)
		if err != nil {
			logger.Log.Printf("NIC: %s, failed to get port data, err: %+v", nic.ID, err)
			continue
		}
		var resp Response
		err = json.Unmarshal(portResp, &resp)
		if err != nil {
			logger.Log.Printf("NIC: %s, error unmarshalling port data: %v", nic.ID, err)
			continue
		}

		for _, nic := range resp.NIC {
			nics[nic.ID].Ports = map[string]*Port{}
			for index, port := range nic.Port {
				nics[nic.ID].Ports[port.Spec.ID] = &Port{
					Index: fmt.Sprintf("%v", index),
					UUID:  port.Spec.ID,
					Name:  port.Spec.Name,
				}
			}
		}
	}

	// fetch lif details for each NIC
	for _, nic := range resp.NIC {
		cmd := fmt.Sprintf("nicctl show lif --card %s -j", nic.ID)
		lifResp, err := ExecWithContext(cmd)
		if err != nil {
			logger.Log.Printf("NIC: %s, failed to get lif data, err: %+v", nic.ID, err)
			continue
		}
		var resp Response
		err = json.Unmarshal(lifResp, &resp)
		if err != nil {
			logger.Log.Printf("NIC: %s, error unmarshalling lif data: %v", nic.ID, err)
			continue
		}

		for _, nic := range resp.NIC {
			lifIndex := 0
			nics[nic.ID].Lifs = map[string]*Lif{}
			for index, lif := range nic.Lif {
				nics[nic.ID].Lifs[lif.Spec.ID] = &Lif{
					Index: fmt.Sprintf("%v", lifIndex),
					UUID:  lif.Spec.ID,
					Name:  lif.Status.Name,
				}
				lifIndex++

				// assume first LIF is the PF and
				// card's ethBDF is the PCIe address for the PF
				if index == 0 {
					nics[nic.ID].Lifs[lif.Spec.ID].IsPF = true
					nics[nic.ID].Lifs[lif.Spec.ID].PCIeAddress = nics[nic.ID].EthBDF
				} else {
					pcieAddr, err := na.getPCIeAddress(nics[nic.ID].EthBDF)
					if err != nil || pcieAddr == "" {
						logger.Log.Printf("NIC: %s, failed to get PCIe address for LIF: %s, err: %v. Health monitoring will be skipped for this LIF",
							nic.ID, lif.Status.Name, err)
						continue
					}
					// VF, congiured on both NIC and host
					nics[nic.ID].sriovConfiguredOnHost = true
					nics[nic.ID].Lifs[lif.Spec.ID].PCIeAddress = pcieAddr
				}
			}
		}
	}
	return nics, nil
}

// populateStaticHostLabels populates static host labels for NIC metrics
func (na *NICAgentClient) populateStaticHostLabels() error {
	na.staticHostLabels = map[string]string{}
	hostname, err := utils.GetHostName()
	if err != nil {
		return err
	}
	na.staticHostLabels[exportermetrics.NICMetricLabel_NIC_HOSTNAME.String()] = hostname
	return nil
}

// getPCIeAddress retrieves the PCIe address of VF from the given ethBDF.
// The ethBDF is expected to be in the format "0000:84:00
func (na *NICAgentClient) getPCIeAddress(ethBDF string) (string, error) {
	// bdf = 0000:84:00.0
	parts := strings.Split(ethBDF, ":")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid ethBDF format: %s", ethBDF)
	}
	bus := parts[1]
	device := strings.Split(parts[2], ".")[0]
	// combine as "bus.device" for grep
	pfPartialBDF := fmt.Sprintf("%s.%s", bus, device) // 84.00

	// sample lspci output:
	// root@genoa4:~# lspci | grep 84:00
	// 84:00.0 Ethernet controller: Pensando Systems DSC Ethernet Controller
	// 84:00.1 Ethernet controller: Pensando Systems DSC Ethernet Controller VF
	// root@genoa4:~# lspci | grep 84:00 | awk '{print $1}'
	// 84:00.0
	// 84:00.1
	// root@genoa4:~#
	cmd := fmt.Sprintf("lspci | grep %s | awk '{print $1}'", pfPartialBDF)
	out, err := ExecWithContext(cmd)
	if err != nil {
		logger.Log.Printf("failed to list PCI devices, err: %+v", err)
		return "", err
	}
	pcieAddrs := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(pcieAddrs) < 2 { // 1pf_1vf
		return "", fmt.Errorf("could not find PCIe address for VF")
	}
	pcieAddr := pcieAddrs[1] // take the second line, which is the VF address

	// 84:00.1 -> 0000:84:00.1
	result := fmt.Sprintf("0000:%s", pcieAddr)
	return result, nil
}

func (na *NICAgentClient) printNICs() {
	for nicID, nic := range na.nics {
		fmt.Printf("NIC ID: %s, Product Name: %s, Serial Number: %s, BDF: %s\n", nicID, nic.ProductName, nic.SerialNumber, nic.EthBDF)
		for portID, port := range nic.Ports {
			fmt.Printf("\tPort ID: %s, Name: %s, MAC Address: %s\n", portID, port.Name, port.MACAddress)
		}
		for lifID, lif := range nic.Lifs {
			fmt.Printf("\tLIF ID: %s, Name: %s, PCIe Address: %s, IsPF: %v\n", lifID, lif.Name, lif.PCIeAddress, lif.IsPF)
		}
	}
}
