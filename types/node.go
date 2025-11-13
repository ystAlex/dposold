package types

import (
	"sync"
	"time"
)

// node.go
// DPoS节点数据结构定义

// NodeType 节点类型枚举
type NodeType int

const (
	// VoterNode 投票节点 - 参与投票选举代理节点
	VoterNode NodeType = iota
	// DelegateNode 代理节点 - 负责出块
	DelegateNode
)

// String 实现Stringer接口
func (nt NodeType) String() string {
	switch nt {
	case VoterNode:
		return "Voter"
	case DelegateNode:
		return "Delegate"
	default:
		return "Unknown"
	}
}

// Int 字符串转NodeType
func (nt NodeType) Int(nodeType string) NodeType {
	switch nodeType {
	case "Voter":
		return VoterNode
	case "Delegate":
		return DelegateNode
	default:
		return NodeType(-1)
	}
}

// Node DPoS网络节点
type Node struct {
	mu sync.RWMutex

	// ================================
	// 基本信息
	// ================================
	ID       string   // 节点唯一标识符
	Type     NodeType // 节点类型（投票者/代理者）
	IsActive bool     // 是否在线活跃
	Address  string   // 网络地址

	// ================================
	// 权益信息（质押）
	// ================================
	Stake        float64 // 质押的代币数量（决定投票权重）
	InitialStake float64 // 初始质押量

	// ================================
	// 投票相关
	// ================================
	VotedFor      string    // 投票给的代理节点ID
	VoteTime      time.Time // 投票时间
	ReceivedVotes float64   // 收到的投票权重总和（仅代理节点）
	VoterCount    int       // 投票者数量（仅代理节点）

	// ================================
	// 出块统计（代理节点）
	// ================================
	BlocksProduced   int       // 已生产的区块数
	MissedBlocks     int       // 错过的出块机会
	LastBlockTime    time.Time // 最后一次出块时间
	ConsecutiveTerms int       // 连续当选代理的轮数

	// ================================
	// 奖励统计
	// ================================
	TotalReward    float64   // 累计获得的总奖励
	BlockRewards   float64   // 出块奖励
	VotingRewards  float64   // 投票奖励
	LastRewardTime time.Time // 最后获得奖励的时间

	// ================================
	// 网络参数
	// ================================
	NetworkDelay   float64   // 网络延迟（毫秒）
	LastSeen       time.Time // 最后活跃时间
	LastUpdateTime time.Time // 最后更新时间

	// ================================
	// 历史记录
	// ================================
	VoteHistory  []VoteRecord  // 投票历史
	BlockHistory []BlockRecord // 出块历史

	// ================================
	// 时延测量字段
	// ================================
	VoteStartTime      time.Time // 投票开始时间
	VoteEndTime        time.Time // 投票完成时间
	BlockPropStartTime time.Time // 区块广播开始时间
	BlockPropEndTime   time.Time // 区块接收完成时间
}

// NewNode 创建新节点
func NewNode(id string, stake float64, networkDelay float64) *Node {
	return &Node{
		ID:             id,
		Type:           VoterNode,
		IsActive:       true,
		Stake:          stake,
		InitialStake:   stake,
		NetworkDelay:   networkDelay,
		LastSeen:       time.Now(),
		LastUpdateTime: time.Now(),
		VoteHistory:    make([]VoteRecord, 0),
		BlockHistory:   make([]BlockRecord, 0),
	}
}

// ================================
// 投票相关方法
// ================================

// Vote 投票给指定的代理节点
func (n *Node) Vote(delegateID string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.VoteStartTime = time.Now() // 记录投票开始时间
	n.VotedFor = delegateID
	n.VoteTime = time.Now()

	// 记录投票历史
	n.VoteHistory = append(n.VoteHistory, VoteRecord{
		Timestamp: time.Now(),
		VotedFor:  delegateID,
		Weight:    n.Stake,
		Success:   true,
	})

	// 记录投票完成时间
	n.VoteEndTime = time.Now()
}

// ResetVote 重置投票状态（新一轮开始时调用）
func (n *Node) ResetVote() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.VotedFor = ""
	n.VoteTime = time.Time{}
	n.VoteStartTime = time.Time{} // 重置投票开始时间 ⬅️ 新增
	n.VoteEndTime = time.Time{}   // 重置投票完成时间 ⬅️ 新增
}

// AddReceivedVotes 增加收到的投票权重（代理节点）
func (n *Node) AddReceivedVotes(votes float64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.ReceivedVotes += votes
	n.VoterCount++
}

// ResetReceivedVotes 重置收到的投票（新一轮开始时调用）
func (n *Node) ResetReceivedVotes() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.ReceivedVotes = 0
	n.VoterCount = 0
}

// ================================
// 出块相关方法
// ================================

// RecordBlock 记录出块结果
func (n *Node) RecordBlock(success bool, reward float64, txCount int) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if success {
		n.BlocksProduced++
		n.BlockRewards += reward
		n.TotalReward += reward
		n.LastBlockTime = time.Now()
		n.LastRewardTime = time.Now()
	} else {
		n.MissedBlocks++
	}

	// 记录出块历史
	n.BlockHistory = append(n.BlockHistory, BlockRecord{
		Timestamp:        time.Now(),
		BlockHeight:      n.BlocksProduced + n.MissedBlocks,
		Success:          success,
		Reward:           reward,
		TransactionCount: txCount,
	})
}

// ================================
// 奖励相关方法
// ================================

// AddVotingReward 添加投票奖励
func (n *Node) AddVotingReward(amount float64) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.VotingRewards += amount
	n.TotalReward += amount
	n.LastRewardTime = time.Now()
}

// ================================
// 状态查询方法
// ================================

// GetVotingPower 获取投票权重
func (n *Node) GetVotingPower() float64 {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.Stake
}

// GetBlockSuccessRate 获取出块成功率
func (n *Node) GetBlockSuccessRate() float64 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	total := n.BlocksProduced + n.MissedBlocks
	if total == 0 {
		return 0
	}
	return float64(n.BlocksProduced) / float64(total)
}

// ShouldBeKicked 判断是否应该被踢出代理节点
func (n *Node) ShouldBeKicked() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()

	// 错过太多区块
	if n.MissedBlocks > 10 { // MaxMissedBlocks
		return true
	}

	// 不再活跃
	if !n.IsActive {
		return true
	}

	return false
}

// ================================
// 类型转换方法
// ================================

// PromoteToDelegate 提升为代理节点
func (n *Node) PromoteToDelegate() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.Type = DelegateNode
	n.ConsecutiveTerms++
}

// DemoteToVoter 降级为投票节点
func (n *Node) DemoteToVoter() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.Type = VoterNode
	n.ConsecutiveTerms = 0
}

// Clone 深拷贝节点
func (n *Node) Clone() *Node {
	n.mu.RLock()
	defer n.mu.RUnlock()

	clone := *n

	// 深拷贝切片
	clone.VoteHistory = make([]VoteRecord, len(n.VoteHistory))
	copy(clone.VoteHistory, n.VoteHistory)

	clone.BlockHistory = make([]BlockRecord, len(n.BlockHistory))
	copy(clone.BlockHistory, n.BlockHistory)

	return &clone
}
