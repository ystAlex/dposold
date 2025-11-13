package types

import "time"

// records.go
// DPoS历史记录数据结构

// VoteRecord 投票记录
type VoteRecord struct {
	Timestamp time.Time // 投票时间戳
	RoundID   int       // 所属轮次
	VotedFor  string    // 投票给的代理节点ID
	Weight    float64   // 投票权重（质押量）
	Success   bool      // 是否成功记录
}

// BlockRecord 出块记录
type BlockRecord struct {
	Timestamp        time.Time     // 出块时间戳
	BlockHeight      int           // 区块高度
	RoundID          int           // 所属轮次
	Success          bool          // 是否成功出块
	ProductionTime   time.Duration // 出块耗时
	TransactionCount int           // 包含的交易数
	Reward           float64       // 获得的奖励
}

// PerformanceSnapshot 性能快照
type PerformanceSnapshot struct {
	Timestamp time.Time // 快照时间
	RoundID   int       // 轮次ID
	NodeID    string    // 节点ID
	NodeType  NodeType  // 节点类型

	// 基础数据
	Stake          float64 // 质押量
	ReceivedVotes  float64 // 收到的投票（代理节点）
	BlocksProduced int     // 生产的区块数
	MissedBlocks   int     // 错过的区块数
	TotalReward    float64 // 累计奖励
}

// NetworkSnapshot 网络快照
type NetworkSnapshot struct {
	Timestamp time.Time // 快照时间
	RoundID   int       // 轮次ID

	// 网络统计
	TotalNodes    int     // 总节点数
	ActiveNodes   int     // 活跃节点数
	DelegateNodes int     // 代理节点数
	TotalStake    float64 // 总质押量

	// 投票统计
	TotalVotes          float64 // 总投票权重
	VotingParticipation float64 // 投票参与率

	// 出块统计
	TotalBlocks      int     // 总区块数
	BlockSuccessRate float64 // 出块成功率

	// 奖励统计
	TotalRewardPool float64 // 总奖励池
	AvgBlockReward  float64 // 平均区块奖励
}
