package core

import (
	"dpos/config"
	"dpos/types"
	"dpos/utils"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"
)

// DPoSConsensus DPoS共识引擎
// 负责管理投票、选举和出块的核心逻辑
type DPoSConsensus struct {
	mu sync.RWMutex

	// 网络状态
	Network *types.Network

	// 区块调度器
	Scheduler *BlockScheduler

	// 投票记录 - 记录收到的所有投票
	// key: voterID, value: targetID
	VoteRecords map[string]string

	// 代理节点得票统计
	// key: delegateID, value: 总得票权重
	VoteStats map[string]float64

	// 当前轮次的投票是否已完成
	VotingComplete bool

	// 恶意节点检测
	MaliciousNodes map[string]int // 节点恶意行为计数

	// 日志记录器
	Logger *utils.Logger

	LastVotingStartTime time.Time // 本轮投票开始时间
	LastVotingEndTime   time.Time // 本轮投票结束时间
	LastBlockStartTime  time.Time // 本轮出块开始时间
	LastBlockEndTime    time.Time // 本轮出块结束时间
}

// NewDPoSConsensus 创建DPoS共识引擎
// 参数:
//   - network: 网络实例
//   - logger: 日志记录器
func NewDPoSConsensus(network *types.Network, logger *utils.Logger) *DPoSConsensus {
	return &DPoSConsensus{
		Network:        network,
		Scheduler:      NewBlockScheduler(),
		VoteRecords:    make(map[string]string),
		VoteStats:      make(map[string]float64),
		VotingComplete: false,
		MaliciousNodes: make(map[string]int),
		Logger:         logger,
	}
}

// ================================
// 投票阶段
// ================================

// StartVotingPhase 开始新一轮投票
// 重置本地投票状态，准备接收新的投票
func (d *DPoSConsensus) StartVotingPhase() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Logger.Info("========================================")
	d.Logger.Info("第 %d 轮投票阶段开始", d.Network.CurrentRound+1)
	d.Logger.Info("========================================")

	d.LastVotingStartTime = time.Now() // 记录投票阶段开始时间

	// 重置投票记录和统计
	d.VoteRecords = make(map[string]string)
	d.VoteStats = make(map[string]float64)
	d.VotingComplete = false

	// 重置本地节点的投票状态
	for _, node := range d.Network.GetNodes() {
		node.ResetVote()
		if node.Type == types.DelegateNode {
			node.ResetReceivedVotes()
		}
	}

	d.Logger.Info("投票状态已重置，等待接收投票...")
}

// CastVote 本地节点投票
// 返回投票的目标节点ID
func (d *DPoSConsensus) CastVote(voterNode *types.Node) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 验证节点是否有资格投票
	if !voterNode.IsActive || voterNode.Stake < config.MinVotingStake {
		d.Logger.Debug("节点 %s 不符合投票条件", voterNode.ID)
		return ""
	}

	// 选择要投票的代理节点
	targetID := d.selectBestDelegate(voterNode)
	if targetID == "" {
		d.Logger.Debug("节点 %s 未找到合适的投票目标", voterNode.ID)
		return ""
	}

	// 记录本地投票
	voterNode.Vote(targetID)
	d.Logger.Info("节点 %s 投票给 %s (权重: %.2f)",
		voterNode.ID, targetID, voterNode.Stake)

	return targetID
}

// ReceiveVote 接收来自其他节点的投票
// 参数:
//   - voterID: 投票者节点ID
//   - targetID: 被投票的节点ID
//   - weight: 投票权重(质押量)
//
// 这是真实的P2P投票接收逻辑
func (d *DPoSConsensus) ReceiveVote(voterID, targetID string, weight float64) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 记录投票
	d.VoteRecords[voterID] = targetID

	// 累加得票统计
	d.VoteStats[targetID] += weight

	d.Logger.Debug("收到投票: %s -> %s (权重: %.2f, 累计: %.2f)",
		voterID, targetID, weight, d.VoteStats[targetID])

	// 更新网络中节点的投票状态
	for _, node := range d.Network.GetNodes() {
		if node.ID == voterID {
			node.Vote(targetID)
		}
		if node.ID == targetID {
			node.AddReceivedVotes(weight)
		}
	}
}

// selectBestDelegate 选择最佳投票目标
// 策略: 选择质押量最高且表现良好的候选节点
func (d *DPoSConsensus) selectBestDelegate(voter *types.Node) string {
	candidates := make([]*types.Node, 0)

	// 收集所有候选节点(活跃且质押达标)
	for _, node := range d.Network.GetNodes() {
		if node.ID == voter.ID {
			continue // 不能投给自己
		}
		if node.IsActive && node.Stake >= config.MinVotingStake {
			candidates = append(candidates, node)
		}
	}

	if len(candidates) == 0 {
		return ""
	}

	//// 按质押量排序，选择最高的
	//sort.Slice(candidates, func(i, j int) bool {
	//	// 优先考虑质押量
	//	if candidates[i].Stake != candidates[j].Stake {
	//		return candidates[i].Stake > candidates[j].Stake
	//	}
	//	// 质押相同时，考虑出块成功率
	//	return candidates[i].GetBlockSuccessRate() > candidates[j].GetBlockSuccessRate()
	//})
	//随机
	rand.Seed(int64(time.Now().Nanosecond()))
	candidate := candidates[rand.Intn(len(candidates))]

	return candidate.ID
}

// MarkVotingComplete 标记投票阶段完成
// 在投票时间窗口结束后调用
func (d *DPoSConsensus) MarkVotingComplete() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.LastVotingEndTime = time.Now() // 记录投票阶段结束时间
	d.VotingComplete = true
	d.calculateVotingStats()

	votingDuration := d.LastVotingEndTime.Sub(d.LastVotingStartTime).Milliseconds()
	d.Logger.Info("投票阶段完成 (耗时: %d ms)", votingDuration)
	d.Logger.Info("  总投票数: %d", len(d.VoteRecords))
	d.Logger.Info("  投票参与率: %.2f%%", d.Network.VotingParticipation*100)
}

// calculateVotingStats 计算投票统计数据
func (d *DPoSConsensus) calculateVotingStats() {
	totalStake := 0.0
	totalVotes := 0.0

	for _, node := range d.Network.GetNodes() {
		if node.IsActive {
			totalStake += node.Stake
			if node.VotedFor != "" {
				totalVotes += node.Stake
			}
		}
	}

	participation := 0.0
	if totalStake > 0 {
		participation = totalVotes / totalStake
	}

	d.Network.UpdateStats(types.NetworkStats{
		TotalStake:          totalStake,
		TotalVotes:          totalVotes,
		ActiveNodesCount:    d.countActiveNodes(),
		VotingParticipation: participation,
	})
}

// countActiveNodes 统计活跃节点数量
func (d *DPoSConsensus) countActiveNodes() int {
	count := 0
	for _, node := range d.Network.GetNodes() {
		if node.IsActive {
			count++
		}
	}
	return count
}

// ================================
// 代理节点选举 - 分布式共识
// ================================

// ElectDelegates 基于收到的投票选举代理节点
// 每个节点独立计算选举结果，结果应该一致(确定性)
func (d *DPoSConsensus) ElectDelegates() []*types.Node {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.Logger.Info("========================================")
	d.Logger.Info("代理节点选举(基于 %d 张投票)", len(d.VoteRecords))
	d.Logger.Info("========================================")

	// 收集候选节点并设置得票数
	candidates := d.collectAndRankCandidates()

	// 选出前N个节点
	numDelegates := config.NumDelegates
	if len(candidates) < numDelegates {
		numDelegates = len(candidates)
	}

	delegates := candidates[:numDelegates]

	// 更新节点类型
	d.updateNodeTypes(delegates)

	// 更新网络状态
	d.Network.SetDelegates(delegates)

	// 更新调度器
	d.Scheduler.UpdateDelegates(delegates)

	// 打印选举结果
	d.printElectionResults(delegates)

	return delegates
}

// collectAndRankCandidates 收集候选节点并按得票排序
func (d *DPoSConsensus) collectAndRankCandidates() []*types.Node {
	candidates := make([]*types.Node, 0)

	// 收集所有活跃且未被踢出的节点
	for _, node := range d.Network.GetNodes() {
		if !node.IsActive || node.ShouldBeKicked() {
			continue
		}

		// 从VoteStats获取真实的得票数
		if votes, exists := d.VoteStats[node.ID]; exists {
			node.ReceivedVotes = votes
		}

		candidates = append(candidates, node)
	}

	// 按得票数排序(确定性排序，保证所有节点结果一致)
	sort.Slice(candidates, func(i, j int) bool {
		// 优先按得票数排序
		if candidates[i].ReceivedVotes != candidates[j].ReceivedVotes {
			return candidates[i].ReceivedVotes > candidates[j].ReceivedVotes
		}
		// 得票相同时按质押量排序
		if candidates[i].Stake != candidates[j].Stake {
			return candidates[i].Stake > candidates[j].Stake
		}
		// 最后按节点ID排序(保证确定性)
		return candidates[i].ID < candidates[j].ID
	})

	return candidates
}

// updateNodeTypes 更新节点类型(当选/未当选)
func (d *DPoSConsensus) updateNodeTypes(delegates []*types.Node) {
	// 构建当选节点ID集合
	delegateIDs := make(map[string]bool)
	for _, delegate := range delegates {
		delegateIDs[delegate.ID] = true
		delegate.PromoteToDelegate()
	}

	// 将未当选的节点降级为投票节点
	for _, node := range d.Network.GetNodes() {
		if !delegateIDs[node.ID] && node.Type == types.DelegateNode {
			node.DemoteToVoter()
		}
	}
}

// printElectionResults 打印选举结果
func (d *DPoSConsensus) printElectionResults(delegates []*types.Node) {
	d.Logger.Info("选举结果：共选出 %d 个代理节点", len(delegates))
	for i, delegate := range delegates {
		d.Logger.Info("  #%d: %s (得票: %.2f, 质押: %.2f, 连任: %d轮)",
			i+1, delegate.ID, delegate.ReceivedVotes,
			delegate.Stake, delegate.ConsecutiveTerms)
	}
}

// ================================
// 出块阶段 - 基于时间槽的确定性调度
// ================================

// GetCurrentProducer 获取当前时隙的出块者
// 基于全局时间和代理节点列表确定性计算
func (d *DPoSConsensus) GetCurrentProducer() (*types.Node, bool) {
	return d.Scheduler.GetCurrentProducer()
}

// ProduceBlock 生产区块
// 参数:
//   - producer: 出块节点
//   - txCount: 交易数量
//
// 返回: 是否成功出块
func (d *DPoSConsensus) ProduceBlock(producer *types.Node, txCount int) bool {
	d.LastBlockStartTime = time.Now() // 记录出块开始时间

	d.Logger.Info(">>> 节点 %s 开始出块 (轮次: %d, 时隙: %d/%d)",
		producer.ID,
		d.Network.CurrentRound,
		d.Scheduler.CurrentSlot+1,
		len(d.Network.Delegates))

	// 模拟出块过程
	success := true

	// 计算奖励
	reward := d.calculateBlockReward(txCount)

	// 记录出块结果
	producer.RecordBlock(success, reward, txCount)

	d.LastBlockEndTime = time.Now() // 记录出块结束时间

	if success {
		d.Logger.Info("✓ 区块生产成功 (奖励: %.2f)", reward)

		// 分配奖励
		d.distributeRewards(producer, reward)

		// 更新区块高度
		d.Scheduler.CurrentHeight++
		d.Network.CurrentBlockHeight++
	} else {
		d.Logger.Warn("✗ 区块生产失败")
	}

	return success
}

// calculateBlockReward 计算区块奖励
func (d *DPoSConsensus) calculateBlockReward(txCount int) float64 {
	// 基础奖励
	reward := config.BlockReward

	// 交易手续费
	txFees := float64(txCount) * 0.01

	return reward + txFees
}

// distributeRewards 分配区块奖励
// 出块者获得大部分，投票者分享小部分
func (d *DPoSConsensus) distributeRewards(producer *types.Node, totalReward float64) {
	// 出块者获得的奖励
	producerReward := totalReward * config.TransactionFeeRatio

	// 投票者分享的奖励
	voterReward := totalReward * config.VoterRewardRatio

	// 分配给投票者
	d.distributeVoterRewards(producer, voterReward)

	d.Logger.Debug("  奖励分配: 出块者=%.2f, 投票者池=%.2f",
		producerReward, voterReward)
}

// distributeVoterRewards 分配投票者奖励
// 按投票权重比例分配
func (d *DPoSConsensus) distributeVoterRewards(producer *types.Node, totalReward float64) {
	if producer.ReceivedVotes == 0 {
		return
	}

	// 遍历所有投票给该出块者的节点
	for _, voter := range d.Network.GetNodes() {
		if voter.VotedFor == producer.ID {
			// 按投票权重比例分配
			reward := totalReward * (voter.Stake / producer.ReceivedVotes)
			voter.AddVotingReward(reward)

			d.Logger.Debug("    投票者 %s 获得奖励: %.2f", voter.ID, reward)
		}
	}
}

// NextSlot 前进到下一个时隙
func (d *DPoSConsensus) NextSlot() {
	d.Scheduler.NextSlot()

	// 检查是否完成一轮
	if d.Scheduler.CurrentSlot == 0 {
		d.Network.CurrentRound++
		d.Logger.Info("========== 第 %d 轮开始 ==========", d.Network.CurrentRound)
	}
}

// ================================
// 状态查询
// ================================

// GetStatus 获取共识状态摘要
func (d *DPoSConsensus) GetStatus() string {
	producer, ok := d.GetCurrentProducer()
	producerID := "无"
	if ok && producer != nil {
		producerID = producer.ID
	}

	return fmt.Sprintf(
		"轮次: %d | 区块高度: %d | 代理节点: %d | 当前出块者: %s | 时隙: %d/%d",
		d.Network.CurrentRound,
		d.Network.CurrentBlockHeight,
		len(d.Network.Delegates),
		producerID,
		d.Scheduler.CurrentSlot+1,
		len(d.Network.Delegates),
	)
}

// GetNetworkSnapshot 创建网络状态快照
func (d *DPoSConsensus) GetNetworkSnapshot() types.NetworkSnapshot {
	totalBlocks := 0
	successfulBlocks := 0

	for _, node := range d.Network.Delegates {
		totalBlocks += node.BlocksProduced + node.MissedBlocks
		successfulBlocks += node.BlocksProduced
	}

	blockSuccessRate := 0.0
	if totalBlocks > 0 {
		blockSuccessRate = float64(successfulBlocks) / float64(totalBlocks)
	}

	return types.NetworkSnapshot{
		Timestamp:           utils.TimeNow(),
		RoundID:             d.Network.CurrentRound,
		TotalNodes:          len(d.Network.Nodes),
		ActiveNodes:         d.Network.ActiveNodesCount,
		DelegateNodes:       len(d.Network.Delegates),
		TotalStake:          d.Network.TotalStake,
		TotalVotes:          d.Network.TotalVotes,
		VotingParticipation: d.Network.VotingParticipation,
		TotalBlocks:         totalBlocks,
		BlockSuccessRate:    blockSuccessRate,
		TotalRewardPool:     d.Network.TotalRewardPool,
	}
}

// GetVoteRecords 获取投票记录副本
func (d *DPoSConsensus) GetVoteRecords() map[string]string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	records := make(map[string]string)
	for k, v := range d.VoteRecords {
		records[k] = v
	}
	return records
}

// GetVoteStats 获取得票统计副本
func (d *DPoSConsensus) GetVoteStats() map[string]float64 {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stats := make(map[string]float64)
	for k, v := range d.VoteStats {
		stats[k] = v
	}
	return stats
}
