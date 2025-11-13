package network

import (
	"dpos/p2p"
	"dpos/types"
	"dpos/utils"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ================================
// 同步请求和响应数据结构
// ================================

// SyncRequest 区块同步请求
// 用于节点间请求缺失的区块
type SyncRequest struct {
	FromID     string    `json:"from_id"`     // 发起请求的节点ID
	ToID       string    `json:"to_id"`       // 目标节点ID
	FromHeight int       `json:"from_height"` // 请求的起始区块高度
	ToHeight   int       `json:"to_height"`   // 请求的结束区块高度(0表示最新)
	RequestID  string    `json:"request_id"`  // 请求唯一标识符
	Timestamp  time.Time `json:"timestamp"`   // 请求时间戳
}

// SyncResponse 区块同步响应
// 包含请求的区块数据
type SyncResponse struct {
	FromID       string    `json:"from_id"`       // 响应节点ID
	ToID         string    `json:"to_id"`         // 请求节点ID
	RequestID    string    `json:"request_id"`    // 对应的请求ID
	Blocks       []*Block  `json:"blocks"`        // 返回的区块列表
	LatestHeight int       `json:"latest_height"` // 响应节点的最新区块高度
	HasMore      bool      `json:"has_more"`      // 是否还有更多区块
	Timestamp    time.Time `json:"timestamp"`     // 响应时间戳
}

// SyncManager 同步管理器
// 负责协调节点间的区块同步
type SyncManager struct {
	LocalNode       *types.Node             // 本地节点
	BlockPool       *BlockPool              // 区块池
	Logger          *utils.Logger           // 日志记录器
	PendingRequests map[string]*SyncRequest // 待处理的同步请求(key: requestID)
	SyncTimeout     time.Duration           // 同步超时时间
	mu              sync.RWMutex            // 读写锁
}

// NewSyncManager 创建同步管理器实例
// 参数:
//   - localNode: 本地节点
//   - blockPool: 区块池
//   - logger: 日志记录器
func NewSyncManager(localNode *types.Node, blockPool *BlockPool, logger *utils.Logger) *SyncManager {
	return &SyncManager{
		LocalNode:       localNode,
		BlockPool:       blockPool,
		Logger:          logger,
		PendingRequests: make(map[string]*SyncRequest),
		SyncTimeout:     30 * time.Second,
	}
}

// RequestSync 请求同步区块
// 参数:
//   - nodeID: 本地节点ID
//   - targetNodeID: 目标节点ID
//   - fromHeight: 起始高度
//   - toHeight: 结束高度(0表示同步到最新)
//   - transport: 传输层
//   - peerManager: 对等节点管理器
//
// 返回:
//   - int: 同步到的最新高度
//   - error: 错误信息
func (sm *SyncManager) RequestSync(
	nodeID, targetNodeID string,
	fromHeight, toHeight int,
	transport *p2p.HTTPTransport,
	peerManager *p2p.PeerManager,
) (int, error) {
	// 创建唯一的请求ID
	requestID := fmt.Sprintf("sync-%s-%d", nodeID, time.Now().Unix())

	// 构造同步请求
	syncReq := &SyncRequest{
		FromID:     nodeID,
		ToID:       targetNodeID,
		FromHeight: fromHeight,
		ToHeight:   toHeight,
		RequestID:  requestID,
		Timestamp:  time.Now(),
	}

	// 记录待处理请求
	sm.mu.Lock()
	sm.PendingRequests[requestID] = syncReq
	sm.mu.Unlock()

	sm.Logger.Info("节点 %s 向 %s 请求同步区块 [%d - %d]",
		nodeID, targetNodeID, fromHeight, toHeight)

	// 获取目标节点信息
	peer, exists := peerManager.GetPeer(targetNodeID)
	if !exists {
		return 0, fmt.Errorf("目标节点 %s 不存在", targetNodeID)
	}

	// 准备请求数据
	payload := map[string]interface{}{
		"from_id":     syncReq.FromID,
		"to_id":       syncReq.ToID,
		"from_height": syncReq.FromHeight,
		"to_height":   syncReq.ToHeight,
		"request_id":  syncReq.RequestID,
		"timestamp":   syncReq.Timestamp,
	}

	// 发送同步请求
	resp, err := transport.SendJSON(peer.Address, "/sync_request", payload)
	if err != nil {
		return 0, fmt.Errorf("发送同步请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 解析同步响应
	var syncResp SyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&syncResp); err != nil {
		return 0, fmt.Errorf("解析同步响应失败: %v", err)
	}

	// 处理同步响应
	syncHeight, err := sm.HandleSyncResponse(syncResp, transport, peerManager)
	if err != nil {
		sm.Logger.Warn("处理同步响应失败: %v", err)
	}

	return syncHeight, nil
}

// HandleSyncRequest 处理收到的同步请求
// 参数:
//   - syncReq: 同步请求
//
// 返回:
//   - []byte: 序列化的响应数据
//   - error: 错误信息
func (sm *SyncManager) HandleSyncRequest(syncReq SyncRequest) ([]byte, error) {
	// 从区块池获取请求的区块
	blocks, latestHeight, hasMore := sm.getBlocksForSync(&syncReq)

	// 创建响应
	syncResp := &SyncResponse{
		FromID:       sm.LocalNode.ID,
		ToID:         syncReq.FromID,
		RequestID:    syncReq.RequestID,
		Blocks:       blocks,
		LatestHeight: latestHeight,
		HasMore:      hasMore,
		Timestamp:    time.Now(),
	}

	// 序列化响应数据
	payload, err := json.Marshal(syncResp)
	if err != nil {
		return nil, fmt.Errorf("序列化同步响应失败: %v", err)
	}

	sm.Logger.Info("向 %s 发送同步响应，包含 %d 个区块", syncReq.FromID, len(blocks))

	return payload, nil
}

// getBlocksForSync 获取用于同步的区块
// 参数:
//   - req: 同步请求
//
// 返回:
//   - []*Block: 区块列表
//   - int: 最新高度
//   - bool: 是否还有更多区块
func (sm *SyncManager) getBlocksForSync(req *SyncRequest) ([]*Block, int, bool) {
	const maxBlocksPerResponse = 100 // 每次最多返回100个区块

	blocks := make([]*Block, 0)
	latestBlock := sm.BlockPool.GetLatestBlock()

	// 如果没有区块，返回空列表
	if latestBlock == nil {
		return blocks, 0, false
	}

	latestHeight := latestBlock.Height

	// 确定实际的结束高度
	endHeight := req.ToHeight
	if endHeight == 0 || endHeight > latestHeight {
		endHeight = latestHeight
	}

	// 限制返回的区块数量(避免单次响应过大)
	if endHeight-req.FromHeight > maxBlocksPerResponse {
		endHeight = req.FromHeight + maxBlocksPerResponse
	}

	// 获取区块范围
	for height := req.FromHeight; height <= endHeight; height++ {
		block, err := sm.BlockPool.GetBlock(height)
		if err != nil {
			sm.Logger.Warn("获取区块 %d 失败: %v", height, err)
			break
		}
		blocks = append(blocks, block)
	}

	// 判断是否还有更多区块
	hasMore := endHeight < latestHeight

	return blocks, latestHeight, hasMore
}

// HandleSyncResponse 处理收到的同步响应
// 参数:
//   - syncResp: 同步响应
//   - transport: 传输层
//   - peerManager: 对等节点管理器
//
// 返回:
//   - int: 同步到的最新高度
//   - error: 错误信息
func (sm *SyncManager) HandleSyncResponse(
	syncResp SyncResponse,
	transport *p2p.HTTPTransport,
	peerManager *p2p.PeerManager,
) (int, error) {
	sm.Logger.Info("收到来自 %s 的同步响应，包含 %d 个区块",
		syncResp.FromID, len(syncResp.Blocks))

	// 验证请求是否存在
	sm.mu.RLock()
	req, exists := sm.PendingRequests[syncResp.RequestID]
	sm.mu.RUnlock()

	if !exists {
		sm.Logger.Warn("收到未知请求ID的同步响应: %s", syncResp.RequestID)
		return 0, nil
	}

	// 应用收到的区块
	successCount := 0
	for _, block := range syncResp.Blocks {
		if err := sm.applyBlock(block); err != nil {
			sm.Logger.Warn("应用区块 %d 失败: %v", block.Height, err)
			continue
		}
		successCount++
	}

	sm.Logger.Info("成功同步 %d/%d 个区块", successCount, len(syncResp.Blocks))

	syncHeight := syncResp.LatestHeight

	// 如果还有更多区块，继续请求
	if syncResp.HasMore {
		nextFromHeight := req.FromHeight + len(syncResp.Blocks)
		sm.Logger.Info("继续请求更多区块，起始高度: %d", nextFromHeight)

		// 递归请求剩余区块
		newSyncHeight, err := sm.RequestSync(
			syncResp.ToID,
			syncResp.FromID,
			nextFromHeight,
			req.ToHeight,
			transport,
			peerManager,
		)

		if err != nil {
			sm.Logger.Warn("继续同步失败: %v", err)
		} else if newSyncHeight > syncHeight {
			syncHeight = newSyncHeight
		}
	} else {
		sm.Logger.Info("区块同步完成，当前高度: %d", syncResp.LatestHeight)
	}

	// 清理已完成的请求
	sm.mu.Lock()
	delete(sm.PendingRequests, syncResp.RequestID)
	sm.mu.Unlock()

	return syncHeight, nil
}

// applyBlock 应用区块到本地区块池
// 参数:
//   - block: 要应用的区块
//
// 返回错误如果区块验证失败或添加失败
func (sm *SyncManager) applyBlock(block *Block) error {
	// 验证区块
	if err := sm.validateBlock(block); err != nil {
		return fmt.Errorf("区块验证失败: %v", err)
	}

	// 添加到区块池
	if err := sm.BlockPool.AddBlock(block); err != nil {
		return fmt.Errorf("添加区块失败: %v", err)
	}

	sm.Logger.Debug("成功应用区块 #%d (生产者: %s)", block.Height, block.ProducerID)

	return nil
}

// validateBlock 验证区块有效性
// 参数:
//   - block: 要验证的区块
//
// 验证项:
// 1. 区块高度连续性
// 2. 前一个区块哈希正确性
// 3. 时间戳合理性
// 4. 生产者ID存在性
func (sm *SyncManager) validateBlock(block *Block) error {
	// 1. 验证区块高度连续性
	latestBlock := sm.BlockPool.GetLatestBlock()
	if latestBlock != nil && block.Height != latestBlock.Height+1 {
		return fmt.Errorf("区块高度不连续: 期望 %d, 实际 %d",
			latestBlock.Height+1, block.Height)
	}

	// 2. 验证前一个区块哈希
	if latestBlock != nil && block.PrevHash != latestBlock.Hash {
		return fmt.Errorf("前一个区块哈希不匹配")
	}

	// 3. 验证时间戳(不能在未来)
	if block.Timestamp.After(time.Now().Add(time.Minute)) {
		return fmt.Errorf("区块时间戳异常：在未来")
	}

	// 4. 验证生产者ID
	if block.ProducerID == "" {
		return fmt.Errorf("区块缺少生产者ID")
	}

	return nil
}

// CheckAndSync 检查是否需要同步并自动执行
// 参数:
//   - nodeID: 本地节点ID
//   - peerNodeIDs: 对等节点ID列表
//   - localHeight: 本地区块高度
//   - newestHeight: 网络最新高度
//   - transport: 传输层
//   - peerManager: 对等节点管理器
//
// 返回:
//   - int: 同步到的高度
//   - error: 错误信息
func (sm *SyncManager) CheckAndSync(
	nodeID string,
	peerNodeIDs []string,
	localHeight int,
	newestHeight int,
	transport *p2p.HTTPTransport,
	peerManager *p2p.PeerManager,
) (int, error) {
	// 如果落后于网络，触发同步
	if newestHeight-localHeight > 0 {
		sm.Logger.Info("节点 %s 落后 %d 个区块，触发同步",
			nodeID, newestHeight-localHeight)

		// 检查是否有可用的对等节点
		if len(peerNodeIDs) == 0 {
			return 0, fmt.Errorf("没有可用的对等节点")
		}

		// 选择第一个对等节点作为同步源
		targetPeer := peerNodeIDs[0]
		if targetPeer == nodeID && len(peerNodeIDs) > 1 {
			targetPeer = peerNodeIDs[1]
		}

		// 发起同步请求
		syncHeight, err := sm.RequestSync(nodeID, targetPeer, localHeight+1, 0, transport, peerManager)
		if err != nil {
			return 0, err
		}

		return syncHeight, nil
	}

	return 0, nil
}

// GetSyncStatus 获取同步状态信息
// 返回包含本地高度、待处理请求数等信息的映射
func (sm *SyncManager) GetSyncStatus() map[string]interface{} {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	localLatest := sm.BlockPool.GetLatestBlock()
	localHeight := 0
	if localLatest != nil {
		localHeight = localLatest.Height
	}

	return map[string]interface{}{
		"local_height":     localHeight,
		"pending_requests": len(sm.PendingRequests),
		"is_syncing":       len(sm.PendingRequests) > 0,
	}
}

// CleanupTimedOutRequests 清理超时的同步请求
// 定期调用以清理长时间未响应的请求
func (sm *SyncManager) CleanupTimedOutRequests() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	for reqID, req := range sm.PendingRequests {
		if now.Sub(req.Timestamp) > sm.SyncTimeout {
			sm.Logger.Warn("同步请求 %s 超时，清理", reqID)
			delete(sm.PendingRequests, reqID)
		}
	}
}

// StartPeriodicCleanup 启动定期清理任务
// 每分钟清理一次超时的同步请求
func (sm *SyncManager) StartPeriodicCleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for range ticker.C {
			sm.CleanupTimedOutRequests()
		}
	}()
}
