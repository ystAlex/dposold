package node

import (
	"dpos/config"
	"dpos/core"
	"dpos/network"
	"dpos/p2p"
	"dpos/types"
	"dpos/utils"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"net/http"
	"strings"
	"sync"
	"time"
)

// P2PNode 分布式P2P节点
// 实现完整的DPoS协议，包括:
// - 真实的点对点投票
// - 分布式选举共识
// - 确定性出块调度
// - 区块和交易同步
type P2PNode struct {
	// ================================
	// 本地节点信息
	// ================================
	LocalNode *types.Node // 本地节点实例

	// ================================
	// P2P网络层组件
	// ================================
	PeerManager      *p2p.PeerManager      // 对等节点管理器
	DiscoveryService *p2p.DiscoveryService // 节点发现服务
	Transport        *p2p.HTTPTransport    // HTTP传输层

	// ================================
	// DPoS共识引擎
	// ================================
	Consensus *core.DPoSConsensus // 共识引擎(投票、选举、出块)

	// ================================
	// 网络视图
	// ================================
	Network *types.Network // 全局网络状态视图

	// ================================
	// 数据池
	// ================================
	BlockPool *network.BlockPool       // 区块池
	TxPool    *network.TransactionPool // 交易池

	// ================================
	// 同步管理
	// ================================
	SyncManager *network.SyncManager // 区块同步管理器

	// ================================
	// HTTP服务器
	// ================================
	HTTPServer *http.Server // HTTP API服务器

	// ================================
	// 配置参数
	// ================================
	ListenAddress string    // 监听地址
	SeedNodes     []string  // 种子节点列表
	InitialTime   time.Time // 创世时间

	// ================================
	// 运行状态
	// ================================
	IsRunning bool          // 是否正在运行
	StopChan  chan bool     // 停止信号通道
	Logger    *utils.Logger // 日志记录器

	// ================================
	// 投票阶段控制
	// ================================
	votingPhaseActive bool       // 当前是否在投票阶段
	votingPhaseMu     sync.Mutex // 投票阶段锁
}

// NewP2PNode 创建P2P节点实例
// 参数:
//   - nodeID: 节点唯一标识符
//   - stake: 质押代币数量
//   - networkDelay: 网络延迟(毫秒)
//   - listenAddress: HTTP监听地址
//   - seedNodes: 种子节点地址列表
//   - initialTime: 创世时间
//   - logger: 日志记录器
func NewP2PNode(
	nodeID string,
	stake, networkDelay float64,
	listenAddress string,
	seedNodes []string,
	initialTime time.Time,
	logger *utils.Logger,
) *P2PNode {
	// 创建本地节点
	localNode := types.NewNode(nodeID, stake, networkDelay)
	localNode.Address = listenAddress

	// 创建网络实例
	network1 := types.NewNetwork()
	network1.AddNode(localNode)

	// 创建数据池
	blockPool := network.NewBlockPool()
	txPool := network.NewTransactionPool()
	//TODO: 为了方便测试先生成100000条交易到池子里面去。
	for i := 0; i < 1000000; i++ {
		tx := network.Transaction{
			ID:        uuid.New().String(),
			From:      "",
			To:        "",
			Amount:    10,
			Fee:       2,
			Timestamp: time.Now(),
			Signature: nil,
			Nonce:     int64(i),
		}
		txPool.AddTransaction(&tx)
	}

	// 创建P2P组件
	peerManager := p2p.NewPeerManager(nodeID, config.MaxPeers)
	transport := p2p.NewHTTPTransport(10 * time.Second)
	discoveryService := p2p.NewDiscoveryService(
		seedNodes,
		nodeID,
		listenAddress,
		peerManager,
	)

	// 创建同步管理器
	syncManager := network.NewSyncManager(localNode, blockPool, logger)

	// 创建DPoS共识引擎
	consensus := core.NewDPoSConsensus(network1, logger)

	node := &P2PNode{
		LocalNode:         localNode,
		Network:           network1,
		PeerManager:       peerManager,
		DiscoveryService:  discoveryService,
		Transport:         transport,
		Consensus:         consensus,
		BlockPool:         blockPool,
		TxPool:            txPool,
		SyncManager:       syncManager,
		ListenAddress:     listenAddress,
		SeedNodes:         seedNodes,
		InitialTime:       initialTime,
		IsRunning:         false,
		StopChan:          make(chan bool),
		Logger:            logger,
		votingPhaseActive: false,
	}

	return node
}

// Start 启动节点
// 启动流程:
// 1. 启动HTTP服务器
// 2. 启动节点发现
// 3. 等待网络连接
// 4. 同步对等节点状态
// 5. 启动主循环
func (pn *P2PNode) Start() error {
	pn.Logger.Info("启动节点 %s，监听地址: %s", pn.LocalNode.ID, pn.ListenAddress)
	pn.IsRunning = true

	// 1. 启动HTTP服务器
	if err := pn.startHTTPServer(); err != nil {
		return fmt.Errorf("启动HTTP服务器失败: %v", err)
	}

	// 等待HTTP服务器就绪
	time.Sleep(1 * time.Second)
	if !pn.checkHTTPServerReady() {
		return fmt.Errorf("HTTP服务器启动超时")
	}
	pn.Logger.Info("HTTP服务器已就绪")

	// 注册自己为对等节点
	pn.PeerManager.AddPeer(&p2p.PeerInfo{
		NodeID:   pn.LocalNode.ID,
		Address:  pn.ListenAddress,
		LastSeen: time.Now(),
		IsActive: true,
	})

	// 2. 启动节点发现
	pn.DiscoveryService.Start()

	// 3. 等待网络连接
	time.Sleep(20 * time.Second)

	pn.Logger.Info("节点 %s 启动完成，已连接 %d 个对等节点",
		pn.LocalNode.ID,
		pn.PeerManager.GetPeerCount())

	// 4. 同步对等节点状态
	pn.syncPeerStates()

	// 5. 启动主循环
	go pn.mainLoop()

	return nil
}

// Stop 停止节点
func (pn *P2PNode) Stop() {
	pn.Logger.Info("停止节点 %s", pn.LocalNode.ID)
	pn.IsRunning = false
	pn.StopChan <- true

	if pn.HTTPServer != nil {
		pn.HTTPServer.Close()
	}
}

// mainLoop 主循环 - 协调整个DPoS流程
// 流程:
// 1. 等待创世时间
// 2. 先进行第一轮投票和选举
// 3. 然后开始出块循环
func (pn *P2PNode) mainLoop() {
	// 1. 等待创世时间
	genesisTime := pn.InitialTime
	now := time.Now().UTC()
	elapsedSinceGenesis := now.Sub(genesisTime)

	// 如果创世时间未到，等待
	if elapsedSinceGenesis < 0 {
		waitDuration := -elapsedSinceGenesis
		pn.Logger.Info("等待创世时间到达，等待时长: %v", waitDuration)
		time.Sleep(waitDuration)
		now = time.Now().UTC()
		elapsedSinceGenesis = now.Sub(genesisTime)
	}

	pn.Logger.Info("========================================")
	pn.Logger.Info("创世时间已到，开始DPoS流程")
	pn.Logger.Info("========================================")

	// 2. 第一轮投票和选举（必须在出块前完成）
	pn.Logger.Info("【第0轮】启动初始投票和选举")
	pn.startVotingPhase()

	// 等待投票收集
	pn.Logger.Info("等待投票收集...")
	time.Sleep(100 * time.Millisecond)

	// 结束投票并选举
	pn.endVotingPhaseAndElect()

	// 同步网络状态
	pn.syncPeerStates()

	pn.Logger.Info("初始选举完成，代理节点已就绪")
	pn.Logger.Info("========================================")

	// 3. 计算当前应该的区块高度
	now = time.Now().UTC()
	elapsedSinceGenesis = now.Sub(genesisTime)
	currentBlockHeight := int(elapsedSinceGenesis.Seconds()) / config.BlockInterval

	// 4. 创建定时器
	// 区块定时器 - 每个区块间隔触发一次
	nextBlockTime := genesisTime.Add(time.Duration((currentBlockHeight+1)*config.BlockInterval) * time.Second)
	blockTicker := time.NewTimer(time.Until(nextBlockTime))

	// 同步定时器 - 定期同步区块
	syncTicker := time.NewTicker(2 * time.Second)

	// 投票阶段定时器 - 每轮开始时触发（第一轮已经完成，所以设置为下一轮）
	roundDuration := time.Duration(config.NumDelegates*config.BlockInterval) * time.Second
	nextVotingTime := genesisTime.Add(roundDuration)
	votingTicker := time.NewTimer(time.Until(nextVotingTime))

	defer blockTicker.Stop()
	defer syncTicker.Stop()
	defer votingTicker.Stop()

	pn.Logger.Info("主循环启动 | 当前区块高度: %d", currentBlockHeight)

	for pn.IsRunning {
		select {
		case <-votingTicker.C:
			// 新一轮开始 - 启动投票阶段
			pn.Logger.Info("========================================")
			pn.Logger.Info("新一轮开始 - 启动投票阶段")
			pn.Logger.Info("========================================")

			// 启动投票阶段
			pn.startVotingPhase()

			// 等待投票收集 100ms (给足够时间让所有节点广播投票)
			time.Sleep(100 * time.Millisecond)

			// 结束投票阶段并进行选举
			pn.endVotingPhaseAndElect()

			// 同步网络状态
			pn.syncPeerStates()

			// 重置投票定时器到下一轮
			now = time.Now().UTC()
			currentRound := int(now.Sub(genesisTime).Seconds()) / (config.NumDelegates * config.BlockInterval)
			nextVotingTime = genesisTime.Add(time.Duration((currentRound+1)*config.NumDelegates*config.BlockInterval) * time.Second)
			votingTicker.Reset(time.Until(nextVotingTime))

		case <-blockTicker.C:
			// 处理区块周期
			pn.Logger.Info("[ ========= 处理区块 | 高度: %d =========]", currentBlockHeight)

			// 检查是否需要新一轮投票选举
			// 每个代理节点轮次开始时检查
			if pn.Consensus.Scheduler.CurrentSlot == 0 && currentBlockHeight > 0 {
				// 检查是否是新一轮的开始
				blocksPerRound := config.NumDelegates
				if currentBlockHeight%blocksPerRound == 0 {
					pn.Logger.Info("轮次结束，准备下一轮投票选举")
					// 注意：实际的投票会由 votingTicker 触发
					// 这里只是标记轮次变化
				}
			}

			// 执行共识流程(出块)
			pn.processConsensus()

			// 更新区块高度
			currentBlockHeight++

			// 重置定时器
			nextBlockTime = genesisTime.Add(time.Duration((currentBlockHeight+1)*config.BlockInterval) * time.Second)
			blockTicker.Reset(time.Until(nextBlockTime))

		case <-syncTicker.C:
			// 定期同步区块
			pn.checkSync()

		case <-pn.StopChan:
			pn.Logger.Info("主循环停止 | 最终区块高度: %d", currentBlockHeight)
			return
		}
	}
}

// ================================
// 投票阶段处理
// ================================

// startVotingPhase 启动投票阶段
// 1. 重置投票状态
// 2. 本地节点投票
// 3. 广播投票到网络
func (pn *P2PNode) startVotingPhase() {
	pn.votingPhaseMu.Lock()
	pn.votingPhaseActive = true
	pn.votingPhaseMu.Unlock()

	// 启动投票阶段
	pn.Consensus.StartVotingPhase()

	// 本地节点投票
	targetID := pn.Consensus.CastVote(pn.LocalNode)
	if targetID == "" {
		pn.Logger.Warn("本地节点未能投票")
		return
	}

	// 广播投票到网络
	pn.broadcastVote(targetID)
}

// endVotingPhaseAndElect 结束投票阶段并进行选举
func (pn *P2PNode) endVotingPhaseAndElect() {
	pn.votingPhaseMu.Lock()
	pn.votingPhaseActive = false
	pn.votingPhaseMu.Unlock()

	// 标记投票完成
	pn.Consensus.MarkVotingComplete()

	// 进行选举
	pn.Consensus.ElectDelegates()
}

// broadcastVote 广播投票到所有对等节点
func (pn *P2PNode) broadcastVote(targetID string) {
	voteData := map[string]interface{}{
		"voter_id":  pn.LocalNode.ID,
		"target_id": targetID,
		"weight":    pn.LocalNode.Stake,
		"round":     pn.Network.CurrentRound,
		"timestamp": time.Now().Unix(),
	}

	peers := pn.PeerManager.GetActivePeers()
	successCount := 0
	errorCount := 0

	for _, peer := range peers {
		//也发送给自己投票信息
		//if peer.NodeID == pn.LocalNode.ID {
		//	continue
		//}

		_, err := pn.Transport.SendJSON(peer.Address, "/vote", voteData)
		if err != nil {
			pn.Logger.Debug("向 %s 广播投票失败: %v", peer.Address, err)
			errorCount++
		} else {
			successCount++
		}
	}

	pn.Logger.Info("投票广播完成: 成功 %d, 失败 %d", successCount, errorCount)
}

// ================================
// 出块阶段处理
// ================================

// processConsensus 处理共识流程(出块)
func (pn *P2PNode) processConsensus() {
	slotInfo := pn.Consensus.Scheduler.GetSlotInfo()

	pn.Logger.Info("========================================")
	pn.Logger.Info("轮次: %d | 时隙: %d/%d",
		pn.Network.CurrentRound,
		slotInfo["current_slot"].(int)+1,
		slotInfo["total_delegates"])
	pn.Logger.Info("当前出块节点: %s", slotInfo["current_producer"])
	pn.Logger.Info("========================================")

	// 1. 检查是否轮到本节点出块
	if pn.Consensus.Scheduler.ShouldProduce(pn.LocalNode.ID) {
		pn.Logger.Info(">>> 本节点是当前出块者，开始出块 <<<")
		pn.produceBlock()
	} else {
		producer, _ := pn.Consensus.Scheduler.GetCurrentProducer()
		if producer != nil {
			pn.Logger.Debug("等待 %s 出块...", producer.ID)
		}
	}

	// 2. 等待出块完成
	time.Sleep(500 * time.Millisecond)

	// 3. 前进到下一个时隙
	pn.Consensus.NextSlot()

	// 4. 广播节点状态
	pn.broadcastStatus()
}

// produceBlock 生产区块
// 流程:
// 1. 从交易池获取交易
// 2. 创建区块
// 3. 添加到本地区块池
// 4. 执行共识(记录出块并分配奖励)
// 5. 广播区块到网络
func (pn *P2PNode) produceBlock() {
	startTime := time.Now()

	if !pn.LocalNode.IsActive {
		return
	}

	pn.Logger.Info("【出块阶段】")
	pn.Logger.Info("节点 %s 开始出块", pn.LocalNode.ID)

	// 1. 从交易池获取交易
	txs := pn.TxPool.GetPendingTransactions(100)
	pn.Logger.Info("  从交易池获取 %d 笔交易", len(txs))

	// 2. 创建区块
	prevBlock := pn.BlockPool.GetLatestBlock()
	prevHash := "genesis"
	height := 1
	if prevBlock != nil {
		prevHash = prevBlock.Hash
		height = prevBlock.Height + 1
	}

	block := &network.Block{
		Height:       height,
		ProducerID:   pn.LocalNode.ID,
		Transactions: txs,
		Timestamp:    time.Now(),
		PrevHash:     prevHash,
		Hash:         fmt.Sprintf("hash-%d-%s", time.Now().Unix(), pn.LocalNode.ID),
	}

	// 3. 添加到本地区块池
	if err := pn.BlockPool.AddBlock(block); err != nil {
		pn.Logger.Warn("添加区块失败: %v", err)
		pn.Consensus.ProduceBlock(pn.LocalNode, 0)
		return
	}

	// 计算出块耗时
	productionTime := time.Since(startTime)
	pn.LocalNode.BlockPropStartTime = time.Now() // 记录广播开始时间
	pn.Logger.Info("  出块耗时: %d ms", productionTime.Milliseconds())

	// 4. 执行共识(记录出块并分配奖励)
	pn.Consensus.ProduceBlock(pn.LocalNode, len(txs))

	// 5. 广播区块到网络
	if err := pn.broadcastBlock(block); err != nil {
		pn.Logger.Warn("广播区块失败: %v", err)
		return
	}

	pn.LocalNode.BlockPropEndTime = time.Now() // 记录广播结束时间
	propDuration := pn.LocalNode.BlockPropEndTime.Sub(pn.LocalNode.BlockPropStartTime).Milliseconds()
	pn.Logger.Info("  区块广播耗时: %d ms", propDuration)

	pn.Logger.Info("节点 %s 成功出块 #%d (包含 %d 笔交易)",
		pn.LocalNode.ID, block.Height, len(txs))

	// 6. 移除已打包的交易
	txIDs := extractTxIDs(txs)
	pn.TxPool.RemoveTransactions(txIDs)
}

// ================================
// HTTP服务器 - 处理P2P消息
// ================================

// startHTTPServer 启动HTTP服务器
func (pn *P2PNode) startHTTPServer() error {
	mux := http.NewServeMux()

	// 核心端点
	mux.HandleFunc("/vote", pn.handleVoteRequest)               // 接收投票
	mux.HandleFunc("/block", pn.handleBlockRequest)             // 接收区块
	mux.HandleFunc("/transaction", pn.handleTransactionRequest) // 接收交易

	// 同步端点
	mux.HandleFunc("/sync_request", pn.handleSyncRequestMessage) // 同步请求

	// 节点管理端点
	mux.HandleFunc("/status", pn.handleStatusRequest)       // 状态查询
	mux.HandleFunc("/status_update", pn.handleStatusUpdate) // 状态更新
	mux.HandleFunc("/peers", pn.handlePeersRequest)         // 对等节点列表
	mux.HandleFunc("/heartbeat", pn.handleHeartbeat)        // 心跳

	pn.HTTPServer = &http.Server{
		Addr:    pn.ListenAddress,
		Handler: mux,
	}

	go func() {
		if err := pn.HTTPServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			pn.Logger.Error("HTTP服务器错误: %v", err)
		}
	}()

	return nil
}

// handleVoteRequest 处理投票消息
// 这是真实的P2P投票接收逻辑
func (pn *P2PNode) handleVoteRequest(w http.ResponseWriter, r *http.Request) {
	var voteData struct {
		VoterID   string  `json:"voter_id"`
		TargetID  string  `json:"target_id"`
		Weight    float64 `json:"weight"`
		Round     int     `json:"round"`
		Timestamp int64   `json:"timestamp"`
	}

	if err := json.NewDecoder(r.Body).Decode(&voteData); err != nil {
		pn.Logger.Warn("解析投票数据失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pn.Logger.Info("【收到投票】轮次 %d | 投票者: %s | 目标: %s | 权重: %.2f",
		voteData.Round, voteData.VoterID, voteData.TargetID, voteData.Weight)

	// 检查是否在投票阶段
	pn.votingPhaseMu.Lock()
	votingActive := pn.votingPhaseActive
	pn.votingPhaseMu.Unlock()

	if !votingActive {
		pn.Logger.Warn("当前不在投票阶段，忽略投票")
		http.Error(w, "不在投票阶段", http.StatusBadRequest)
		return
	}

	// 接收投票(更新共识引擎的投票记录)
	pn.Consensus.ReceiveVote(voteData.VoterID, voteData.TargetID, voteData.Weight)

	w.WriteHeader(http.StatusOK)
}

// handleBlockRequest 处理区块消息
func (pn *P2PNode) handleBlockRequest(w http.ResponseWriter, r *http.Request) {
	var block network.Block
	if err := json.NewDecoder(r.Body).Decode(&block); err != nil {
		pn.Logger.Warn("解析区块数据失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pn.Logger.Info("收到来自 %s 的区块 #%d", block.ProducerID, block.Height)

	// 验证并添加区块
	if err := pn.BlockPool.AddBlock(&block); err != nil {
		if strings.Contains(err.Error(), "已存在") {
			w.WriteHeader(http.StatusOK)
			return
		} else {
			pn.Logger.Warn("添加区块失败: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	// 更新最新区块号
	pn.BlockPool.SetNewestBlockNumber(block.Height)

	// 从交易池移除已确认的交易
	txIDs := make([]string, len(block.Transactions))
	for i, tx := range block.Transactions {
		txIDs[i] = tx.ID
	}
	pn.TxPool.RemoveTransactions(txIDs)

	w.WriteHeader(http.StatusOK)
}

// handleTransactionRequest 处理交易消息
func (pn *P2PNode) handleTransactionRequest(w http.ResponseWriter, r *http.Request) {
	var tx network.Transaction

	if err := json.NewDecoder(r.Body).Decode(&tx); err != nil {
		pn.Logger.Warn("解析交易数据失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pn.Logger.Debug("收到交易 %s: %s -> %s (金额: %.2f)",
		tx.ID, tx.From, tx.To, tx.Amount)

	// 添加到交易池
	if err := pn.TxPool.AddTransaction(&tx); err != nil {
		pn.Logger.Warn("添加交易失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleStatusRequest 处理状态查询
func (pn *P2PNode) handleStatusRequest(w http.ResponseWriter, r *http.Request) {
	status := pn.GetStatus()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleStatusUpdate 处理状态更新
func (pn *P2PNode) handleStatusUpdate(w http.ResponseWriter, r *http.Request) {
	var statusData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&statusData); err != nil {
		pn.Logger.Warn("解析状态数据失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 提取节点信息并更新
	nodeID := statusData["id"].(string)
	stake := statusData["stake"].(float64)
	isActive := statusData["is_active"].(bool)

	// 更新或添加节点
	found := false
	for _, node := range pn.Network.GetNodes() {
		if node.ID == nodeID {
			node.Stake = stake
			node.IsActive = isActive
			found = true
			break
		}
	}

	if !found {
		newNode := types.NewNode(nodeID, stake, 20.0)
		newNode.IsActive = isActive
		pn.Network.AddNode(newNode)
	}

	w.WriteHeader(http.StatusOK)
}

// handlePeersRequest 处理节点列表请求
func (pn *P2PNode) handlePeersRequest(w http.ResponseWriter, r *http.Request) {
	peers := pn.PeerManager.GetActivePeers()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peers)
}

// handleHeartbeat 处理心跳
func (pn *P2PNode) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var heartbeatData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&heartbeatData); err != nil {
		pn.Logger.Warn("解析心跳数据失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	nodeID := heartbeatData["node_id"].(string)
	address := heartbeatData["address"].(string)

	// 记录对等节点
	pn.PeerManager.AddPeer(&p2p.PeerInfo{
		NodeID:   nodeID,
		Address:  address,
		LastSeen: time.Now(),
		IsActive: true,
	})

	// 返回对等列表
	peers := pn.PeerManager.GetActivePeers()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peers)
}

// handleSyncRequestMessage 处理同步请求
func (pn *P2PNode) handleSyncRequestMessage(w http.ResponseWriter, r *http.Request) {
	var syncReq network.SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&syncReq); err != nil {
		pn.Logger.Warn("解析同步请求失败: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	pn.Logger.Info("收到同步请求 [%d - %d]", syncReq.FromHeight, syncReq.ToHeight)

	// 使用SyncManager处理
	msg, err := pn.SyncManager.HandleSyncRequest(syncReq)
	if err != nil {
		pn.Logger.Warn("处理同步请求失败: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(msg)
}

// ================================
// 辅助方法
// ================================

// syncPeerStates 同步对等节点状态
// 从所有对等节点获取最新状态并更新本地网络视图
func (pn *P2PNode) syncPeerStates() {
	peers := pn.PeerManager.GetActivePeers()
	var wg sync.WaitGroup

	for _, peer := range peers {
		wg.Add(1)
		go func(p *p2p.PeerInfo) {
			defer wg.Done()

			// 请求节点状态
			status, err := pn.requestNodeStatus(p.Address)
			if err != nil {
				return
			}

			// 更新节点信息
			found := false
			for _, node := range pn.Network.GetNodes() {
				if node.ID == status.NodeID {
					node.Stake = status.Stake
					node.IsActive = status.IsActive
					node.Type = node.Type.Int(status.NodeType)
					found = true
					break
				}
			}

			// 添加新节点
			if !found {
				newNode := types.NewNode(status.NodeID, status.Stake, 20.0)
				newNode.IsActive = status.IsActive
				newNode.Type = newNode.Type.Int(status.NodeType)
				pn.Network.AddNode(newNode)
			}

			pn.Logger.Debug("同步节点状态: %s (质押: %.2f, 类型: %s)",
				status.NodeID, status.Stake, status.NodeType)
		}(peer)
	}

	wg.Wait()
}

// checkSync 检查并执行区块同步
func (pn *P2PNode) checkSync() {
	localHeight := pn.BlockPool.GetLatestHeight()
	newestHeight := pn.BlockPool.GetNewestBlockNumber()

	if newestHeight-localHeight > 0 {
		peerIDs := make([]string, 0)
		for _, peer := range pn.PeerManager.GetActivePeers() {
			peerIDs = append(peerIDs, peer.NodeID)
		}

		if len(peerIDs) == 0 {
			return
		}

		targetPeer := peerIDs[0]
		if targetPeer == pn.LocalNode.ID && len(peerIDs) > 1 {
			targetPeer = peerIDs[1]
		}

		syncHeight, err := pn.SyncManager.CheckAndSync(
			pn.LocalNode.ID,
			peerIDs,
			localHeight,
			newestHeight,
			pn.Transport,
			pn.PeerManager,
		)

		if err != nil {
			pn.Logger.Warn("同步失败: %v", err)
		} else if syncHeight > 0 {
			pn.Logger.Info("同步到区块 %d", syncHeight)
		} else if syncHeight > 0 {
			pn.Logger.Info("同步到区块 %d", syncHeight)
		}
	}
}

// broadcastBlock 广播区块到所有对等节点
func (pn *P2PNode) broadcastBlock(block *network.Block) error {
	peers := pn.PeerManager.GetActivePeers()
	err := pn.Transport.BroadcastJSON(peers, "/block", block)

	if err != nil {
		pn.Logger.Warn("广播区块失败: %v", err)
	} else {
		pn.Logger.Info("成功广播区块 #%d 到 %d 个节点", block.Height, len(peers))
	}

	return err
}

// broadcastStatus 广播节点状态到所有对等节点
func (pn *P2PNode) broadcastStatus() {
	statusData := map[string]interface{}{
		"id":        pn.LocalNode.ID,
		"type":      pn.LocalNode.Type.String(),
		"is_active": pn.LocalNode.IsActive,
		"stake":     pn.LocalNode.Stake,
		"reward":    pn.LocalNode.TotalReward,
		"timestamp": time.Now().Unix(),
	}

	peers := pn.PeerManager.GetActivePeers()
	for _, peer := range peers {
		go pn.sendStatusUpdate(peer.Address, statusData)
	}
}

// requestNodeStatus 请求指定节点的状态信息
func (pn *P2PNode) requestNodeStatus(address string) (*NodeStatus, error) {
	var status NodeStatus
	err := pn.Transport.GetJSON(address, "/status", &status)
	return &status, err
}

// sendStatusUpdate 发送状态更新到指定节点
func (pn *P2PNode) sendStatusUpdate(address string, statusData map[string]interface{}) error {
	_, err := pn.Transport.SendJSON(address, "/status_update", statusData)
	return err
}

// checkHTTPServerReady 检查HTTP服务器是否就绪
func (pn *P2PNode) checkHTTPServerReady() bool {
	client := &http.Client{Timeout: 1 * time.Second}

	for i := 0; i < 10; i++ {
		resp, err := client.Get(fmt.Sprintf("http://%s/status", pn.ListenAddress))
		if err == nil {
			resp.Body.Close()
			return true
		}
		time.Sleep(500 * time.Millisecond)
	}

	return false
}

// GetStatus 获取节点完整状态
func (pn *P2PNode) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"id":              pn.LocalNode.ID,
		"type":            pn.LocalNode.Type.String(),
		"is_active":       pn.LocalNode.IsActive,
		"stake":           pn.LocalNode.Stake,
		"voted_for":       pn.LocalNode.VotedFor,
		"received_votes":  pn.LocalNode.ReceivedVotes,
		"blocks_produced": pn.LocalNode.BlocksProduced,
		"missed_blocks":   pn.LocalNode.MissedBlocks,
		"total_reward":    pn.LocalNode.TotalReward,
		"block_rewards":   pn.LocalNode.BlockRewards,
		"voting_rewards":  pn.LocalNode.VotingRewards,
		"is_running":      pn.IsRunning,
		"connected_peers": pn.PeerManager.GetPeerCount(),
		"current_round":   pn.Network.CurrentRound,
		"block_height":    pn.BlockPool.GetLatestHeight(),
	}
}

// NodeStatus 节点状态结构体
// 用于网络传输
type NodeStatus struct {
	NodeID         string  `json:"id"`
	NodeType       string  `json:"type"`
	IsActive       bool    `json:"is_active"`
	Stake          float64 `json:"stake"`
	VotedFor       string  `json:"voted_for"`
	ReceivedVotes  float64 `json:"received_votes"`
	BlocksProduced int     `json:"blocks_produced"`
	MissedBlocks   int     `json:"missed_blocks"`
	TotalReward    float64 `json:"total_reward"`
}

// extractTxIDs 从交易列表中提取交易ID
func extractTxIDs(txs []*network.Transaction) []string {
	ids := make([]string, len(txs))
	for i, tx := range txs {
		ids[i] = tx.ID
	}
	return ids
}
