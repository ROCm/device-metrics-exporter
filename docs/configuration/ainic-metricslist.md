# List of Available AINIC Metrics

The following table contains a full list of AINIC Metrics that are available using the Device Metrics Exporter. 

| Metric                                   | Description                                                                 |
|------------------------------------------|-----------------------------------------------------------------------------|
| NIC_NODES_TOTAL                          | Number of NIC nodes on the host                                             |
|                                          |                                                                             |
| --- Port stats ---                       |                                                                             | 
| NIC_PORT_STATS_FRAMES_RX_BAD_FCS         | Total number of frames rcvd with FCS error on a port                        |
| NIC_PORT_STATS_FRAMES_RX_BAD_ALL         | Total number of bad frames received on a port                               |
| NIC_PORT_STATS_FRAMES_RX_PAUSE           | Total number of pause frames received on a network port                     |
| NIC_PORT_STATS_FRAMES_RX_BAD_LENGTH      | Total number of frames received that have an incorrect or invalid length    |
| NIC_PORT_STATS_FRAMES_RX_UNDERSIZED      | Total number of frames rcvd that are smaller than minimum frame size        |
| NIC_PORT_STATS_FRAMES_RX_OVERSIZED       | Total number of frames rcvd that exceed max frame size allowed              |
| NIC_PORT_STATS_FRAMES_RX_FRAGMENTS       | Total number of frames received that are fragments of larger packets        |
| NIC_PORT_STATS_FRAMES_RX_JABBER          | Total number of frames received that are considered jabber frames           |
| NIC_PORT_STATS_FRAMES_RX_PRIPAUSE        | Total number of priority pause frames received                              |
| NIC_PORT_STATS_FRAMES_RX_STOMPED_CRC     | Total number of frames received that had valid CRC but were stomped         |
| NIC_PORT_STATS_FRAMES_RX_TOO_LONG        | Total number of frames rcvd that exceed max allowable size for frames       |
| NIC_PORT_STATS_FRAMES_RX_DROPPED         | Total frames rcvd but dropped due to reasons such as buffer overflows etc   |
| NIC_PORT_STATS_FRAMES_TX_BAD             | Total number of transmitted frames that are considered bad                  |
| NIC_PORT_STATS_FRAMES_TX_PAUSE           | Total number of pause frames transmitted                                    |
| NIC_PORT_STATS_FRAMES_TX_PRIPAUSE        | Total number of priority pause frames transmitted                           |
| NIC_PORT_STATS_FRAMES_TX_LESS_THAN_64B   | Total number of tx frames smaller than min frame size i.e 64 bytes          |
| NIC_PORT_STATS_FRAMES_TX_TRUNCATED       | Total number of frames that were transmitted but truncated                  |
| NIC_PORT_STATS_RSFEC_CORRECTABLE_WORD    | Total number of RS-FEC correctable words received or transmitted            |
| NIC_PORT_STATS_RSFEC_CH_SYMBOL_ERR_CNT   | Total count of channel symbol errors detected by the RS-FEC mechanism       |
|                                          |                                                                             |
| --- LIF (PF/VF) stats ---                |                                                                             |
| NIC_LIF_STATS_RX_UNICAST_PACKETS         | Total number of unicast packets received by the LIF                         |
| NIC_LIF_STATS_RX_UNICAST_DROP_PACKETS    | Number of unicast packets that were dropped during reception                |
| NIC_LIF_STATS_RX_MULTICAST_DROP_PACKETS  | Number of multicast packets that were dropped during reception              |
| NIC_LIF_STATS_RX_BROADCAST_DROP_PACKETS  | Number of broadcast packets that were dropped during reception              |
| NIC_LIF_STATS_RX_DMA_ERRORS              | Number of errors encountered while performing DMA during pkt reception      |
| NIC_LIF_STATS_TX_UNICAST_PACKETS         | Total number of unicast packets transmitted by the LIF                      |
| NIC_LIF_STATS_TX_UNICAST_DROP_PACKETS    | Number of unicast packets that were dropped during transmission             |
| NIC_LIF_STATS_TX_MULTICAST_DROP_PACKETS  | Number of multicast packets that were dropped during transmission           |
| NIC_LIF_STATS_TX_BROADCAST_DROP_PACKETS  | Number of broadcast packets that were dropped during transmission           |
| NIC_LIF_STATS_TX_DMA_ERRORS              | Number of errors encountered while performing DMA during pkt transmission   |
|                                          |                                                                             |
| --- RDMA (RoCE Interface) stats ---      |                                                                             |
| RDMA_TX_UCAST_PKTS                       | Tx RDMA Unicast Packets                                                     |
| RDMA_TX_CNP_PKTS                         | Tx RDMA Congestion Notification Packets                                     |
| RDMA_RX_UCAST_PKTS                       | Rx RDMA Ucast Pkts                                                          |
| RDMA_RX_CNP_PKTS                         | Rx RDMA Congestion Notification Packets                                     |
| RDMA_RX_ECN_PKTS                         | Rx RDMA Explicit Congestion Notification Packets                            |
| RDMA_REQ_RX_PKT_SEQ_ERR                  | Request Rx packet sequence errors                                           |
| RDMA_REQ_RX_RNR_RETRY_ERR                | Request Rx receiver not ready retry errors                                  |
| RDMA_REQ_RX_RMT_ACC_ERR                  | Request Rx remote access errors                                             |
| RDMA_REQ_RX_RMT_REQ_ERR                  | Request Rx remote request errors                                            |
| RDMA_REQ_RX_OPER_ERR                     | Request Rx remote oper errors                                               |
| RDMA_REQ_RX_IMPL_NAK_SEQ_ERR             | Request Rx implicit negative acknowledgment errors                          |
| RDMA_REQ_RX_CQE_ERR                      | Request Rx completion queue errors                                          |
| RDMA_REQ_RX_CQE_FLUSH                    | Request Rx completion queue flush count                                     |
| RDMA_REQ_RX_DUP_RESP                     | Request Rx duplicate response errors                                        |
| RDMA_REQ_RX_INVALID_PKTS                 | Request Rx invalid pkts                                                     |
| RDMA_REQ_TX_LOC_ERR                      | Request Tx local errors                                                     |
| RDMA_REQ_TX_LOC_OPER_ERR                 | Request Tx local operation errors                                           |
| RDMA_REQ_TX_MEM_MGMT_ERR                 | Request Tx memory management errors                                         |
| RDMA_REQ_TX_RETRY_EXCD_ERR               | Request Tx Retry exceeded errors                                            |
| RDMA_REQ_TX_LOC_SGL_INV_ERR              | Request Tx local signal inversion errors                                    |
| RDMA_RESP_RX_DUP_REQUEST                 | Response Rx duplicate request count                                         |
| RDMA_RESP_RX_OUTOF_BUF                   | Response Rx out of buffer count                                             |
| RDMA_RESP_RX_OUTOUF_SEQ                  | Response Rx out of sequence count                                           |
| RDMA_RESP_RX_CQE_ERR                     | Response Rx completion queue errors                                         |
| RDMA_RESP_RX_CQE_FLUSH                   | Response Rx completion queue flush                                          |
| RDMA_RESP_RX_LOC_LEN_ERR                 | Response Rx local length errors                                             |
| RDMA_RESP_RX_INVALID_REQUEST             | Response Rx invalid requests count                                          |
| RDMA_RESP_RX_LOC_OPER_ERR                | Response Rx local operation errors                                          |
| RDMA_RESP_RX_OUTOF_ATOMIC                | Response Rx without atomic guarantee count                                  |
| RDMA_RESP_TX_PKT_SEQ_ERR                 | Response Tx packet sequence error count                                     |
| RDMA_RESP_TX_RMT_INVAL_REQ_ERR           | Response Tx remote invalid request count                                    |
| RDMA_RESP_TX_RMT_ACC_ERR                 | Response Tx remote access error count                                       |
| RDMA_RESP_TX_RMT_OPER_ERR                | Response Tx remote operation error count                                    |
| RDMA_RESP_TX_RNR_RETRY_ERR               | Response Tx retry not required error count                                  |
| RDMA_RESP_TX_LOC_SGL_INV_ERR             | Response Tx local signal inversion error count                              |
| RDMA_RESP_RX_S0_TABLE_ERR                | Response rx S0 Table error count                                            |
|                                          |                                                                             |
| --- RoCE Queue-Pair stats ---            |                                                                             |
|   --- Send Queue Requester stats ---     |                                                                             |
| QP_SQ_REQ_TX_NUM_PACKET                  | SQ Requester Tx Number of Packets                                           |
| QP_SQ_REQ_TX_NUM_SEND_MSGS_WITH_RKE      | SQ Requester Tx Number Send Msg with Remote Key Error                       |
| QP_SQ_REQ_TX_NUM_LOCAL_ACK_TIMEOUTS      | SQ Requester Number of local ACK timeouts for a Tx msg on QP                |
| QP_SQ_REQ_TX_RNR_TIMEOUT                 | SQ Requester Number of send operation timeouts due to Receiver Not Ready NAK|
| QP_SQ_REQ_TX_TIMES_SQ_DRAINED            | SQ Requester Number of times SQ moved to drained state after Tx complete    |
| QP_SQ_REQ_TX_NUM_CNP_SENT                | SQ Requester Number of Congestion Notification Packets  sent for the SQ     |
| QP_SQ_REQ_RX_NUM_PACKET                  | Number of Packets received on SQ                                            |
| QP_SQ_REQ_RX_NUM_PKTS_WITH_ECN_MARKING   | Num Pkts received on SQ with Explicity congestion Notification bit set      |
| QP_SQ_QCN_CURR_BYTE_COUNTER              | Current Byte counter used by Quantized congestion notification algo on SQ   |
| QP_SQ_QCN_NUM_BYTE_COUNTER_EXPIRED       | QCN byte counter threshold hit count for the SQ                             |
| QP_SQ_QCN_NUM_TIMER_EXPIRED              | QCN dedicated timer expiry count for the SQ                                 |
| QP_SQ_QCN_NUM_ALPHA_TIMER_EXPIRED        | QCN Alpha timer expiry count for the SQ                                     |
| QP_SQ_QCN_NUM_CNP_RCVD                   | QCN congestion notification pkt count rcvd on the SQ                        |
| QP_SQ_QCN_NUM_CNP_PROCESSED              | Count of CNPs successfully processed by QCN algo on the SQ                  |
|   --- Receive Queue Responder stats ---  |                                                                             |
| QP_RQ_RSP_TX_NUM_PACKET                  | RQ Responder Tx number of Packets                                           |
| QP_RQ_RSP_TX_RNR_ERROR                   | Count of Receiver Not Ready errors sent by RQ                               |
| QP_RQ_RSP_TX_NUM_SEQUENCE_ERROR          | Count of Negative ACK sent by RQ due to Out of Sequence incoming msg        | 
| QP_RQ_RSP_TX_NUM_RP_BYTE_THRES_HIT       | Number of times RP Byte threshold hit on RQ                                 |
| QP_RQ_RSP_TX_NUM_RP_MAX_RATE_HIT         | Number of times Response Pkt max rate was hit affect Tx responses           |
| QP_RQ_RSP_RX_NUM_PACKET                  | RQ Responder Rx number of Packets                                           |
| QP_RQ_RSP_RX_NUM_SEND_MSGS_WITH_RKE      | RQ Responder count of send mgs with RDMA key                                |
| QP_RQ_RSP_RX_NUM_PKTS_WITH_ECN_MARKING   | Number of packets received on RQ with Explicit Congestion Notification set  |
| QP_RQ_RSP_RX_NUM_CNPS_RECEIVED           | Number of Congestion Notification Packets received on RQ                    |
| QP_RQ_RSP_RX_MAX_RECIRC_EXCEEDED_DROP    | Number of incoming pkts on RQ dropped due to max internal recirculations hit|
| QP_RQ_RSP_RX_NUM_MEM_WINDOW_INVALID      | Number of RDMA operations rejected due to invalid memory window access      |
| QP_RQ_RSP_RX_NUM_DUPL_WITH_WR_SEND_OPC   | Number of incoming duplicate Send operation packets on RQ                   |
| QP_RQ_RSP_RX_NUM_DUPL_READ_BACKTRACK     | Count of duplicate packets that resulted in backtracking of Packet Seq Num  |
| QP_RQ_RSP_RX_NUM_DUPL_READ_ATOMIC_DROP   | Count of duplicate pkts that resulted in drop of RDMA operations            |
| QP_SQ_QCN_CURR_BYTE_COUNTER              | Current Byte counter used by Quantized congestion notification algo on RQ   |
| QP_SQ_QCN_NUM_BYTE_COUNTER_EXPIRED       | QCN byte counter threshold hit count for the RQ                             |
| QP_SQ_QCN_NUM_TIMER_EXPIRED              | QCN dedicated timer expiry count for the RQ                                 |
| QP_SQ_QCN_NUM_ALPHA_TIMER_EXPIRED        | QCN Alpha timer expiry count for the RQ                                     |
| QP_SQ_QCN_NUM_CNP_RCVD                   | QCN congestion notification pkt count rcvd on the RQ                        |
| QP_SQ_QCN_NUM_CNP_PROCESSED              | Count of CNPs successfully processed by QCN algo on the RQ                  |




## Port Stats example

```json
nic_port_stats_frames_rx_bad_all{nic_hostname="ubuntu",nic_id="0",nic_serial_number="FPL25180020",port_name="eth1/1"} 0
nic_port_stats_frames_rx_bad_all{nic_hostname="ubuntu",nic_id="1",nic_serial_number="FPL244500E4",port_name="eth1/1"} 0

nic_port_stats_frames_rx_bad_fcs{nic_hostname="ubuntu",nic_id="0",nic_serial_number="FPL25180020",port_name="eth1/1"} 0
nic_port_stats_frames_rx_bad_fcs{nic_hostname="ubuntu",nic_id="1",nic_serial_number="FPL244500E4",port_name="eth1/1"} 0
```



## LIF Stats example

```json
nic_lif_stats_rx_unicast_drop_packets{lif_name="enp132s0",nic_hostname="ubuntu",nic_id="1",nic_serial_number="FPL244500E4"} 0
nic_lif_stats_rx_unicast_drop_packets{lif_name="enp132s0v0",nic_hostname="ubuntu",nic_id="1",nic_serial_number="FPL244500E4"} 0
nic_lif_stats_rx_unicast_drop_packets{lif_name="enp68s0",nic_hostname="ubuntu",nic_id="0",nic_serial_number="FPL25180020"} 0
nic_lif_stats_rx_unicast_drop_packets{lif_name="eth0_vf1",nic_hostname="ubuntu",nic_id="0",nic_serial_number="FPL25180020"} 0

nic_lif_stats_rx_unicast_packets{lif_name="enp132s0",nic_hostname="ubuntu",nic_id="1",nic_serial_number="FPL244500E4"} 128530
nic_lif_stats_rx_unicast_packets{lif_name="enp132s0v0",nic_hostname="ubuntu",nic_id="1",nic_serial_number="FPL244500E4"} 6.2882736e+07
nic_lif_stats_rx_unicast_packets{lif_name="enp68s0",nic_hostname="ubuntu",nic_id="0",nic_serial_number="FPL25180020"} 22
nic_lif_stats_rx_unicast_packets{lif_name="eth0_vf1",nic_hostname="ubuntu",nic_id="0",nic_serial_number="FPL25180020"} 0
```



## RDMA Stats example

```json
rdma_tx_ucast_pkts{nic_hostname="ubuntu",rdma_if_name="ionic_0"} 3.244137635e+09
rdma_tx_ucast_pkts{nic_hostname="ubuntu",rdma_if_name="roceo3"} 1.6392e+07
rdma_tx_ucast_pkts{nic_hostname="ubuntu",rdma_if_name="rocep132s0"} 0
rdma_tx_ucast_pkts{nic_hostname="ubuntu",rdma_if_name="rocep33s0f0"} 0
rdma_tx_ucast_pkts{nic_hostname="ubuntu",rdma_if_name="rocep33s0f1"} 0

rdma_rx_ucast_pkts{nic_hostname="ubuntu",rdma_if_name="ionic_0"} 3.26044101e+09
rdma_rx_ucast_pkts{nic_hostname="ubuntu",rdma_if_name="roceo3"} 22955
rdma_rx_ucast_pkts{nic_hostname="ubuntu",rdma_if_name="rocep132s0"} 0
rdma_rx_ucast_pkts{nic_hostname="ubuntu",rdma_if_name="rocep33s0f0"} 0
rdma_rx_ucast_pkts{nic_hostname="ubuntu",rdma_if_name="rocep33s0f1"} 0
```
