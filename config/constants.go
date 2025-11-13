package config

// constants.go
// DPoS共识机制配置参数

// ================================
// 代理节点配置
// ================================

const (
	// NumDelegates 代理节点数量
	// DPoS中负责出块的节点数量，通常为奇数以便共识
	NumDelegates = 3

	// BlockInterval 出块间隔（秒）
	// 每个代理节点的出块时间
	BlockInterval = 3

	// RoundDuration 一轮的持续时间（秒）
	// 一轮 = 所有代理节点各出一个块的时间
	RoundDuration = NumDelegates * BlockInterval
)

// ================================
// 投票配置
// ================================

const (
	// VotingPeriod 投票期时长（秒）
	// 每轮开始前的投票时间窗口
	VotingPeriod = 10

	// MinVotingStake 最小投票权重
	// 节点参与投票需要的最低质押量
	MinVotingStake = 1.0

	// VoteUpdateInterval 投票更新间隔（秒）
	// 投票信息的更新频率
	VoteUpdateInterval = 30
)

// ================================
// 奖励配置
// ================================

const (
	// BlockReward 固定出块奖励
	// 每个区块的基础奖励
	BlockReward = 10.0

	// TransactionFeeRatio 交易手续费分配比例
	// 出块者获得的手续费比例
	TransactionFeeRatio = 0.8

	// VoterRewardRatio 投票者奖励比例
	// 投票者分享的区块奖励比例
	VoterRewardRatio = 0.2
)

// ================================
// 惩罚配置
// ================================

const (
	// MissedBlockPenalty 错过出块的惩罚
	// 从下一轮奖励中扣除的比例
	MissedBlockPenalty = 0.1

	// MaxMissedBlocks 最大允许错过的区块数
	// 超过此数量将被剔除代理节点资格
	MaxMissedBlocks = 10
)

// ================================
// 网络配置
// ================================

const (
	// MaxPeers 最大对等节点数
	MaxPeers = 100

	// MaxBlocksPerSync 每次同步最多获取的区块数
	MaxBlocksPerSync = 100

	// SyncInterval 区块同步间隔（秒）
	SyncInterval = 5
)

// ================================
// 系统配置
// ================================

const (
	// SimulationRounds 模拟运行轮数
	SimulationRounds = 50

	// InitialNodesCount 初始节点数量
	InitialNodesCount = 50

	// LogLevel 日志级别
	// 0: ERROR, 1: WARN, 2: INFO, 3: DEBUG
	LogLevel = 2
)
