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
	"strings"
	"sync"

	"github.com/ROCm/device-metrics-exporter/pkg/amdnic/gen/nicmetrics"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/logger"
	"github.com/ROCm/device-metrics-exporter/pkg/exporter/scheduler"
)

type RDMAStatsClient struct {
	sync.Mutex
	na        *NICAgentClient
	rdmaIfMap map[string]string
}

func newRDMAStatsClient(na *NICAgentClient) *RDMAStatsClient {
	nc := &RDMAStatsClient{na: na}
	return nc
}

func (rc *RDMAStatsClient) Init() error {
	rc.Lock()
	defer rc.Unlock()
	rc.rdmaIfMap = make(map[string]string)
	return nil
}
func (rc *RDMAStatsClient) IsActive() bool {
	rc.Lock()
	defer rc.Unlock()
	if _, err := exec.LookPath(RDMABinary); err == nil {
		return true
	}
	return false
}

func (rc *RDMAStatsClient) GetClientName() string {
	return RDMAClientName
}

func (rc *RDMAStatsClient) checkAndUpdateRdmaIfMap(rdmaIf string) error {
	if _, ok := rc.rdmaIfMap[rdmaIf]; !ok {
		cmd := fmt.Sprintf("cat /sys/class/infiniband/%s/device/uevent  | grep PCI_SLOT", rdmaIf)
		out, err := ExecWithContext(cmd)
		if err != nil {
			return fmt.Errorf("could not find PCIe addr for %s: %s", rdmaIf, err)
		}
		parts := strings.Split(strings.TrimSpace(string(out)), "=")
		if len(parts) < 2 || parts[1] == "" {
			return fmt.Errorf("could not find PCIe addr for %s", rdmaIf)
		}
		rc.rdmaIfMap[rdmaIf] = parts[1]
		logger.Log.Printf("adding rdmaIfmap entry %s:%s", rdmaIf, rc.rdmaIfMap[rdmaIf])
	}
	return nil
}

func (rc *RDMAStatsClient) UpdateNICStats(workloads map[string]scheduler.Workload) error {
	if !fetchRdmaMetrics {
		return nil
	}
	rc.Lock()
	defer rc.Unlock()
	res, err := exec.Command("rdma", "statistic", "-j").CombinedOutput()
	if err != nil {
		logger.Log.Printf("RDMA cmd failure err :%v", err)
		return err
	}
	var rdmaStats []nicmetrics.RDMAStats
	err = json.Unmarshal(res, &rdmaStats)
	if err != nil {
		logger.Log.Printf("error unmarshaling rdma statistics data: %v", err)
		return err
	}
	var nicID string
	for _, nic := range rc.na.nics {
		//TODO..Once lif code is committed, need to match rdma netdev and nic's lif name to find nicID
		nicID = nic.UUID
		break
	}
	labels := rc.na.populateLabelsFromNIC(nicID)
	for i := range rdmaStats {
		if err := rc.checkAndUpdateRdmaIfMap(rdmaStats[i].IFNAME); err != nil {
			return err
		}
		rdmaIfName := rdmaStats[i].IFNAME
		rdmaIfPcieAddr := rc.rdmaIfMap[rdmaIfName]
		labels[LabelRdmaIfName] = rdmaIfName
		labels[LabelRdmaIfPcieAddr] = rdmaIfPcieAddr
		workloadLabels := rc.na.getAssociatedWorkloadLabelsForPcieAddr(rdmaIfPcieAddr, workloads)
		for k, v := range workloadLabels {
			labels[k] = v
		}
		rc.na.m.rdmaTxUcastPkts.With(labels).Set(float64(rdmaStats[i].RDMA_TX_UCAST_PKTS))
		rc.na.m.rdmaTxCnpPkts.With(labels).Set(float64(rdmaStats[i].RDMA_TX_CNP_PKTS))
		rc.na.m.rdmaRxUcastPkts.With(labels).Set(float64(rdmaStats[i].RDMA_RX_UCAST_PKTS))
		rc.na.m.rdmaRxCnpPkts.With(labels).Set(float64(rdmaStats[i].RDMA_RX_CNP_PKTS))
		rc.na.m.rdmaRxEcnPkts.With(labels).Set(float64(rdmaStats[i].RDMA_RX_ECN_PKTS))

		rc.na.m.rdmaReqRxPktSeqErr.With(labels).Set(float64(rdmaStats[i].RDMA_REQ_RX_PKT_SEQ_ERR))
		rc.na.m.rdmaReqRxRnrRetryErr.With(labels).Set(float64(rdmaStats[i].RDMA_REQ_RX_RNR_RETRY_ERR))
		rc.na.m.rdmaReqRxRmtAccErr.With(labels).Set(float64(rdmaStats[i].RDMA_REQ_RX_RMT_ACC_ERR))
		rc.na.m.rdmaReqRxRmtReqErr.With(labels).Set(float64(rdmaStats[i].RDMA_REQ_RX_RMT_REQ_ERR))
		rc.na.m.rdmaReqRxOperErr.With(labels).Set(float64(rdmaStats[i].RDMA_REQ_RX_OPER_ERR))
		rc.na.m.rdmaReqRxImplNakSeqErr.With(labels).Set(float64(rdmaStats[i].RDMA_REQ_RX_IMPL_NAK_SEQ_ERR))
		rc.na.m.rdmaReqRxCqeErr.With(labels).Set(float64(rdmaStats[i].RDMA_REQ_RX_CQE_ERR))
		rc.na.m.rdmaReqRxCqeFlush.With(labels).Set(float64(rdmaStats[i].RDMA_REQ_RX_CQE_FLUSH))
		rc.na.m.rdmaReqRxDupResp.With(labels).Set(float64(rdmaStats[i].RDMA_REQ_RX_DUP_RESP))
		rc.na.m.rdmaReqRxInvalidPkts.With(labels).Set(float64(rdmaStats[i].RDMA_REQ_RX_INVALID_PKTS))

		rc.na.m.rdmaReqTxLocErr.With(labels).Set(float64(rdmaStats[i].RDMA_REQ_TX_LOC_ERR))
		rc.na.m.rdmaReqTxLocOperErr.With(labels).Set(float64(rdmaStats[i].RDMA_REQ_TX_LOC_OPER_ERR))
		rc.na.m.rdmaReqTxMemMgmtErr.With(labels).Set(float64(rdmaStats[i].RDMA_REQ_TX_MEM_MGMT_ERR))
		rc.na.m.rdmaReqTxRetryExcdErr.With(labels).Set(float64(rdmaStats[i].RDMA_REQ_TX_RETRY_EXCD_ERR))
		rc.na.m.rdmaReqTxLocSglInvErr.With(labels).Set(float64(rdmaStats[i].RDMA_REQ_TX_LOC_SGL_INV_ERR))

		rc.na.m.rdmaRespRxDupRequest.With(labels).Set(float64(rdmaStats[i].RDMA_RESP_RX_DUP_REQUEST))
		rc.na.m.rdmaRespRxOutofBuf.With(labels).Set(float64(rdmaStats[i].RDMA_RESP_RX_OUTOF_BUF))
		rc.na.m.rdmaRespRxOutoufSeq.With(labels).Set(float64(rdmaStats[i].RDMA_RESP_RX_OUTOUF_SEQ))
		rc.na.m.rdmaRespRxCqeErr.With(labels).Set(float64(rdmaStats[i].RDMA_RESP_RX_CQE_ERR))
		rc.na.m.rdmaRespRxCqeFlush.With(labels).Set(float64(rdmaStats[i].RDMA_RESP_RX_CQE_FLUSH))
		rc.na.m.rdmaRespRxLocLenErr.With(labels).Set(float64(rdmaStats[i].RDMA_RESP_RX_LOC_LEN_ERR))
		rc.na.m.rdmaRespRxInvalidRequest.With(labels).Set(float64(rdmaStats[i].RDMA_RESP_RX_INVALID_REQUEST))
		rc.na.m.rdmaRespRxLocOperErr.With(labels).Set(float64(rdmaStats[i].RDMA_RESP_RX_LOC_OPER_ERR))
		rc.na.m.rdmaRespRxOutofAtomic.With(labels).Set(float64(rdmaStats[i].RDMA_RESP_RX_OUTOF_ATOMIC))

		rc.na.m.rdmaRespTxPktSeqErr.With(labels).Set(float64(rdmaStats[i].RDMA_RESP_TX_PKT_SEQ_ERR))
		rc.na.m.rdmaRespTxRmtInvalReqErr.With(labels).Set(float64(rdmaStats[i].RDMA_RESP_TX_RMT_INVAL_REQ_ERR))
		rc.na.m.rdmaRespTxRmtAccErr.With(labels).Set(float64(rdmaStats[i].RDMA_RESP_TX_RMT_ACC_ERR))
		rc.na.m.rdmaRespTxRmtOperErr.With(labels).Set(float64(rdmaStats[i].RDMA_RESP_TX_RMT_OPER_ERR))
		rc.na.m.rdmaRespTxRnrRetryErr.With(labels).Set(float64(rdmaStats[i].RDMA_RESP_TX_RNR_RETRY_ERR))
		rc.na.m.rdmaRespTxLocSglInvErr.With(labels).Set(float64(rdmaStats[i].RDMA_RESP_TX_LOC_SGL_INV_ERR))

		rc.na.m.rdmaRespRxS0TableErr.With(labels).Set(float64(rdmaStats[i].RDMA_RESP_RX_S0_TABLE_ERR))
	}
	return nil
}
