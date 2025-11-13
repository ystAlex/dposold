package types

import (
	"sync"
)

// network.go
// DPoS网络数据结构定义

// Network DPoS网络
type Network struct {
	mu sync.RWMutex

	// ================================
	// 节点管理
	// ================================
	Nodes     []*Node // 所有节点列表
	Delegates []*Node // 当前代理节点列表

	// ================================
	// 轮次管理
	// ================================
	CurrentRound       int // 当前轮次
	CurrentBlockHeight int // 当前区块高度

	// ================================
	// 统计数据
	// ================================
	TotalStake          float64 // 全网总质押量
	TotalVotes          float64 // 本轮总投票权重
	ActiveNodesCount    int     // 活跃节点数
	VotingParticipation float64 // 投票参与率

	// ================================
	// 奖励统计
	// ================================
	TotalRewardPool   float64 // 总奖励池
	BlockRewardsPool  float64 // 出块奖励池
	VotingRewardsPool float64 // 投票奖励池

	// ================================
	// 历史记录
	// ================================
	SnapshotHistory []NetworkSnapshot // 网络快照历史
}

// NewNetwork 创建新网络实例
func NewNetwork() *Network {
	return &Network{
		Nodes:           make([]*Node, 0),
		Delegates:       make([]*Node, 0),
		CurrentRound:    0,
		SnapshotHistory: make([]NetworkSnapshot, 0),
	}
}

// ===== 线程安全的访问方法 =====

// AddNode 添加节点（线程安全）
func (n *Network) AddNode(node *Node) {
	n.mu.Lock()
	defer n.mu.Unlock()

	// 检查是否已存在
	for i, existingNode := range n.Nodes {
		if existingNode.ID == node.ID {
			n.Nodes[i] = node
			return
		}
	}

	n.Nodes = append(n.Nodes, node)
}

// GetNodes 获取所有节点（线程安全）
func (n *Network) GetNodes() []*Node {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.Nodes
}

// UpdateNode 更新节点（线程安全）
func (n *Network) UpdateNode(nodeID string, node *Node) bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	for i, existingNode := range n.Nodes {
		if existingNode.ID == nodeID {
			n.Nodes[i] = node
			return true
		}
	}

	return false
}

// RemoveNode 移除节点（线程安全）
func (n *Network) RemoveNode(nodeID string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	for i, node := range n.Nodes {
		if node.ID == nodeID {
			n.Nodes = append(n.Nodes[:i], n.Nodes[i+1:]...)
			return
		}
	}
}

// SetDelegates 设置代理节点（线程安全）
func (n *Network) SetDelegates(delegates []*Node) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Delegates = delegates
}

// GetDelegates 获取代理节点（线程安全）
func (n *Network) GetDelegates() []*Node {
	n.mu.RLock()
	defer n.mu.RUnlock()

	delegates := make([]*Node, len(n.Delegates))
	copy(delegates, n.Delegates)
	return delegates
}

// UpdateStats 更新网络统计（线程安全）
func (n *Network) UpdateStats(stats NetworkStats) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.TotalStake = stats.TotalStake
	n.TotalVotes = stats.TotalVotes
	n.ActiveNodesCount = stats.ActiveNodesCount
	n.VotingParticipation = stats.VotingParticipation
}

// GetStats 获取网络统计（线程安全）
func (n *Network) GetStats() NetworkStats {
	n.mu.RLock()
	defer n.mu.RUnlock()

	return NetworkStats{
		TotalStake:          n.TotalStake,
		TotalVotes:          n.TotalVotes,
		ActiveNodesCount:    n.ActiveNodesCount,
		VotingParticipation: n.VotingParticipation,
	}
}

// IncrementRound 增加轮次（线程安全）
func (n *Network) IncrementRound() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.CurrentRound++
}

// IncrementBlockHeight 增加区块高度（线程安全）
func (n *Network) IncrementBlockHeight() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.CurrentBlockHeight++
}

// AddSnapshot 添加网络快照（线程安全）
func (n *Network) AddSnapshot(snapshot NetworkSnapshot) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.SnapshotHistory = append(n.SnapshotHistory, snapshot)
}

// GetLatestSnapshot 获取最新快照（线程安全）
func (n *Network) GetLatestSnapshot() *NetworkSnapshot {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if len(n.SnapshotHistory) == 0 {
		return nil
	}

	return &n.SnapshotHistory[len(n.SnapshotHistory)-1]
}

// GetSnapshotHistory 获取快照历史（线程安全）
func (n *Network) GetSnapshotHistory() []NetworkSnapshot {
	n.mu.RLock()
	defer n.mu.RUnlock()

	history := make([]NetworkSnapshot, len(n.SnapshotHistory))
	copy(history, n.SnapshotHistory)
	return history
}

// NetworkStats 网络统计数据
type NetworkStats struct {
	TotalStake          float64 // 总质押量
	TotalVotes          float64 // 总投票权重
	ActiveNodesCount    int     // 活跃节点数
	VotingParticipation float64 // 投票参与率
}

// ================================
// 辅助方法
// ================================

// GetNodeByID 根据ID获取节点（线程安全）
func (n *Network) GetNodeByID(nodeID string) *Node {
	n.mu.RLock()
	defer n.mu.RUnlock()

	for _, node := range n.Nodes {
		if node.ID == nodeID {
			return node
		}
	}

	return nil
}

// GetActiveNodes 获取所有活跃节点（线程安全）
func (n *Network) GetActiveNodes() []*Node {
	n.mu.RLock()
	defer n.mu.RUnlock()

	activeNodes := make([]*Node, 0)
	for _, node := range n.Nodes {
		if node.IsActive {
			activeNodes = append(activeNodes, node)
		}
	}

	return activeNodes
}

// GetDelegateByID 根据ID获取代理节点（线程安全）
func (n *Network) GetDelegateByID(nodeID string) *Node {
	n.mu.RLock()
	defer n.mu.RUnlock()

	for _, delegate := range n.Delegates {
		if delegate.ID == nodeID {
			return delegate
		}
	}

	return nil
}

// IsDelegate 判断节点是否为代理节点（线程安全）
func (n *Network) IsDelegate(nodeID string) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()

	for _, delegate := range n.Delegates {
		if delegate.ID == nodeID {
			return true
		}
	}

	return false
}

// GetNodeCount 获取节点总数（线程安全）
func (n *Network) GetNodeCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.Nodes)
}

// GetDelegateCount 获取代理节点数量（线程安全）
func (n *Network) GetDelegateCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.Delegates)
}

// CalculateTotalStake 计算总质押量（线程安全）
func (n *Network) CalculateTotalStake() float64 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	total := 0.0
	for _, node := range n.Nodes {
		if node.IsActive {
			total += node.Stake
		}
	}

	return total
}

// CalculateTotalVotes 计算总投票权重（线程安全）
func (n *Network) CalculateTotalVotes() float64 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	total := 0.0
	for _, node := range n.Nodes {
		if node.IsActive && node.VotedFor != "" {
			total += node.Stake
		}
	}

	return total
}

// CalculateVotingParticipation 计算投票参与率（线程安全）
func (n *Network) CalculateVotingParticipation() float64 {
	n.mu.RLock()
	defer n.mu.RUnlock()

	totalStake := 0.0
	votedStake := 0.0

	for _, node := range n.Nodes {
		if node.IsActive {
			totalStake += node.Stake
			if node.VotedFor != "" {
				votedStake += node.Stake
			}
		}
	}

	if totalStake == 0 {
		return 0
	}

	return votedStake / totalStake
}

// Reset 重置网络状态（用于测试）
func (n *Network) Reset() {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.Nodes = make([]*Node, 0)
	n.Delegates = make([]*Node, 0)
	n.CurrentRound = 0
	n.CurrentBlockHeight = 0
	n.TotalStake = 0
	n.TotalVotes = 0
	n.ActiveNodesCount = 0
	n.VotingParticipation = 0
	n.TotalRewardPool = 0
	n.BlockRewardsPool = 0
	n.VotingRewardsPool = 0
	n.SnapshotHistory = make([]NetworkSnapshot, 0)
}
