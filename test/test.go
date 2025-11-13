package test

import (
	"dpos/config"
	"dpos/network"
	"dpos/node"
	"dpos/types"
	"dpos/utils"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ================================
// 测试器主结构
// ================================

type DPoSTester struct {
	Node      *node.P2PNode
	Logger    *utils.Logger
	OutputDir string

	// 测试结果收集
	RoundResults        []RoundResult         // 每轮基础数据
	ThroughputData      []ThroughputData      // 吞吐量数据
	LatencyData         []LatencyData         // 时延数据
	BlockProductionData []BlockProductionData // 区块生产占比
	VotingSpeedData     []VotingSpeedData     // 投票速率
	MaliciousNodeData   []MaliciousNodeData   // 恶意节点数据

	StartTime    time.Time
	TestRounds   int
	CurrentRound int
	mu           sync.RWMutex
}

// ================================
// 数据结构定义
// ================================

// RoundResult 每轮基础数据
type RoundResult struct {
	Round          int
	Timestamp      time.Time
	NodeID         string
	NodeType       string
	Stake          float64
	ReceivedVotes  float64
	BlocksProduced int
	MissedBlocks   int
	TotalReward    float64
	IsActive       bool
}

// ThroughputData 吞吐量数据 (对应论文4.1)
type ThroughputData struct {
	Round           int
	Timestamp       time.Time
	TotalNodes      int     // 节点数量
	TPS             float64 // 每秒交易数
	BlocksPerMinute float64 // 每分钟区块数
	AvgBlockTime    float64 // 平均出块时间(秒)
}

// LatencyData 时延数据 (对应论文4.1)
type LatencyData struct {
	Round            int
	Timestamp        time.Time
	TotalNodes       int
	AvgConsensusTime float64 // 平均共识时间(ms)
	AvgVotingTime    float64 // 平均投票时间(ms)
	AvgBlockPropTime float64 // 平均区块传播时间(ms)
}

// BlockProductionData 区块生产占比 (对应论文4.2)
type BlockProductionData struct {
	Round          int
	Timestamp      time.Time
	NodeID         string
	NodeRank       int     // 节点排名(按得票)
	BlocksProduced int     // 本轮生产区块数
	TotalBlocks    int     // 网络总区块数
	ProductionRate float64 // 生产占比
}

// VotingSpeedData 投票速率 (对应论文4.3)
type VotingSpeedData struct {
	Round         int
	Timestamp     time.Time
	VoteLatency   int64  // 投票耗时(ms)
	LatencyBucket string // 耗时区间: 0-25, 25-50, 50-75, 75-100, >100
	NodeID        string
}

// MaliciousNodeData 恶意节点数据 (对应论文4.4)
type MaliciousNodeData struct {
	Round              int
	Timestamp          time.Time
	MaliciousVoters    int // 恶意投票节点数
	MaliciousDelegates int // 恶意代理节点数
	TotalMalicious     int // 总恶意节点数
}

// ================================
// 构造函数
// ================================

func NewDPoSTester(p2pNode *node.P2PNode, logger *utils.Logger, outputDir string) *DPoSTester {
	return &DPoSTester{
		Node:                p2pNode,
		Logger:              logger,
		OutputDir:           outputDir,
		RoundResults:        make([]RoundResult, 0),
		ThroughputData:      make([]ThroughputData, 0),
		LatencyData:         make([]LatencyData, 0),
		BlockProductionData: make([]BlockProductionData, 0),
		VotingSpeedData:     make([]VotingSpeedData, 0),
		MaliciousNodeData:   make([]MaliciousNodeData, 0),
		StartTime:           time.Now(),
	}
}

// ================================
// 主测试流程
// ================================

func (dt *DPoSTester) RunFullTest(rounds int) {
	dt.TestRounds = rounds
	dt.Logger.Info("========================================")
	dt.Logger.Info("开始 DPoS 性能测试 (原始DPoS方案)")
	dt.Logger.Info("测试轮数: %d", rounds)
	dt.Logger.Info("节点ID: %s", dt.Node.LocalNode.ID)
	dt.Logger.Info("========================================")

	if err := os.MkdirAll(dt.OutputDir, 0755); err != nil {
		dt.Logger.Error("创建输出目录失败: %v", err)
		return
	}

	// 记录初始状态
	dt.collectRoundData(0)

	// 运行测试轮次
	for round := 1; round <= rounds; round++ {
		dt.CurrentRound = round
		dt.Logger.Info("========== 测试轮次 %d/%d ==========", round, rounds)

		// 等待一个轮次完成
		time.Sleep(time.Duration(config.RoundDuration) * time.Second)

		// 收集数据
		dt.collectRoundData(round)

		// 每10轮输出进度
		if round%10 == 0 {
			dt.Logger.Info("测试进度: %d/%d (%.1f%%)",
				round, rounds, float64(round)/float64(rounds)*100)
		}
	}

	// 生成所有报告
	dt.generateAllReports()

	dt.Logger.Info("========================================")
	dt.Logger.Info("✓ 测试完成! 总耗时: %v", time.Since(dt.StartTime))
	dt.Logger.Info("结果保存在: %s", dt.OutputDir)
	dt.Logger.Info("========================================")
}

func (dt *DPoSTester) RunBasicTest(rounds int) {
	dt.TestRounds = rounds
	dt.Logger.Info("开始基础测试，轮数: %d", rounds)

	if err := os.MkdirAll(dt.OutputDir, 0755); err != nil {
		dt.Logger.Error("创建输出目录失败: %v", err)
		return
	}

	dt.collectRoundData(0)

	for round := 1; round <= rounds; round++ {
		dt.CurrentRound = round
		time.Sleep(time.Duration(config.RoundDuration) * time.Second)
		dt.collectRoundData(round)

		if round%10 == 0 {
			dt.Logger.Info("测试进度: %d/%d", round, rounds)
		}
	}

	dt.generateBasicReports()
	dt.Logger.Info("✓ 基础测试完成! 耗时: %v", time.Since(dt.StartTime))
}

// ================================
// 数据收集
// ================================

func (dt *DPoSTester) collectRoundData(round int) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	// 1. 基础轮次数据
	dt.collectBasicRoundData(round)

	// 2. 吞吐量数据
	dt.collectThroughputData(round)

	// 3. 时延数据
	dt.collectLatencyData(round)

	// 4. 区块生产占比
	dt.collectBlockProductionData(round)

	// 5. 投票速率
	dt.collectVotingSpeedData(round)

	// 6. 恶意节点统计
	dt.collectMaliciousNodeData(round)
}

// 1. 基础轮次数据
func (dt *DPoSTester) collectBasicRoundData(round int) {
	localNode := dt.Node.LocalNode

	result := RoundResult{
		Round:          round,
		Timestamp:      time.Now(),
		NodeID:         localNode.ID,
		NodeType:       localNode.Type.String(),
		Stake:          localNode.Stake,
		ReceivedVotes:  localNode.ReceivedVotes,
		BlocksProduced: localNode.BlocksProduced,
		MissedBlocks:   localNode.MissedBlocks,
		TotalReward:    localNode.TotalReward,
		IsActive:       localNode.IsActive,
	}

	dt.RoundResults = append(dt.RoundResults, result)
}

// 2. 吞吐量数据
func (dt *DPoSTester) collectThroughputData(round int) {
	recentBlocks := dt.getRecentBlocks(60)

	totalTxs := 0
	for _, block := range recentBlocks {
		totalTxs += len(block.Transactions)
	}

	tps := float64(totalTxs) / 60.0
	bpm := float64(len(recentBlocks))
	avgBlockTime := 0.0
	if len(recentBlocks) > 0 {
		avgBlockTime = 60.0 / float64(len(recentBlocks))
	}

	totalNodes := len(dt.Node.Network.GetNodes())

	data := ThroughputData{
		Round:           round,
		Timestamp:       time.Now(),
		TotalNodes:      totalNodes,
		TPS:             tps,
		BlocksPerMinute: bpm,
		AvgBlockTime:    avgBlockTime,
	}

	dt.ThroughputData = append(dt.ThroughputData, data)
}

// 3. 时延数据
func (dt *DPoSTester) collectLatencyData(round int) {
	consensus := dt.Node.Consensus
	localNode := dt.Node.LocalNode

	// 1. 真实的投票时延 (从投票开始到投票完成)
	avgVotingTime := 0.0
	if !consensus.LastVotingStartTime.IsZero() && !consensus.LastVotingEndTime.IsZero() {
		avgVotingTime = float64(consensus.LastVotingEndTime.Sub(consensus.LastVotingStartTime).Milliseconds())
	}

	// 2. 真实的出块时延 (从开始出块到完成出块)
	avgBlockTime := 0.0
	if !consensus.LastBlockStartTime.IsZero() && !consensus.LastBlockEndTime.IsZero() {
		avgBlockTime = float64(consensus.LastBlockEndTime.Sub(consensus.LastBlockStartTime).Milliseconds())
	}

	// 3. 真实的区块传播时延 (从广播开始到广播完成)
	avgBlockPropTime := 0.0
	if !localNode.BlockPropStartTime.IsZero() && !localNode.BlockPropEndTime.IsZero() {
		avgBlockPropTime = float64(localNode.BlockPropEndTime.Sub(localNode.BlockPropStartTime).Milliseconds())
	}

	// 4. 计算整体共识时延 (投票 + 出块)
	avgConsensusTime := avgVotingTime + avgBlockTime

	totalNodes := len(dt.Node.Network.GetNodes())

	data := LatencyData{
		Round:            round,
		Timestamp:        time.Now(),
		TotalNodes:       totalNodes,
		AvgConsensusTime: avgConsensusTime,
		AvgVotingTime:    avgVotingTime,
		AvgBlockPropTime: avgBlockPropTime,
	}

	dt.LatencyData = append(dt.LatencyData, data)

	dt.Logger.Debug("[时延] 共识: %.2f ms, 投票: %.2f ms, 广播: %.2f ms",
		avgConsensusTime, avgVotingTime, avgBlockPropTime)
}

// 4. 区块生产占比
func (dt *DPoSTester) collectBlockProductionData(round int) {
	delegates := dt.Node.Network.GetDelegates()
	if len(delegates) == 0 {
		return
	}

	// 计算总区块数
	totalBlocks := 0
	for _, delegate := range delegates {
		totalBlocks += delegate.BlocksProduced
	}

	// 按得票排序节点
	rankedNodes := make([]*types.Node, len(delegates))
	copy(rankedNodes, delegates)

	// 简单排序
	for i := 0; i < len(rankedNodes); i++ {
		for j := i + 1; j < len(rankedNodes); j++ {
			if rankedNodes[j].ReceivedVotes > rankedNodes[i].ReceivedVotes {
				rankedNodes[i], rankedNodes[j] = rankedNodes[j], rankedNodes[i]
			}
		}
	}

	// 记录每个节点的生产占比
	for rank, node := range rankedNodes {
		productionRate := 0.0
		if totalBlocks > 0 {
			productionRate = float64(node.BlocksProduced) / float64(totalBlocks)
		}

		data := BlockProductionData{
			Round:          round,
			Timestamp:      time.Now(),
			NodeID:         node.ID,
			NodeRank:       rank + 1,
			BlocksProduced: node.BlocksProduced,
			TotalBlocks:    totalBlocks,
			ProductionRate: productionRate,
		}

		dt.BlockProductionData = append(dt.BlockProductionData, data)
	}
}

// 5. 投票速率
func (dt *DPoSTester) collectVotingSpeedData(round int) {
	for _, node := range dt.Node.Network.GetNodes() {
		if node.VotedFor == "" {
			continue
		}

		// ✅ 使用真实的投票时延 (VoteEndTime - VoteStartTime)
		voteLatency := int64(0)
		if !node.VoteStartTime.IsZero() && !node.VoteEndTime.IsZero() {
			voteLatency = node.VoteEndTime.Sub(node.VoteStartTime).Milliseconds()
		}

		// 如果没有记录时间戳，跳过该节点
		if voteLatency == 0 {
			continue
		}

		// 根据延迟分桶
		latencyBucket := dt.getLatencyBucket(voteLatency)

		data := VotingSpeedData{
			Round:         round,
			Timestamp:     time.Now(),
			VoteLatency:   voteLatency,
			LatencyBucket: latencyBucket,
			NodeID:        node.ID,
		}

		dt.VotingSpeedData = append(dt.VotingSpeedData, data)
	}
}

// 6. 恶意节点统计
func (dt *DPoSTester) collectMaliciousNodeData(round int) {
	maliciousVoters := 0
	maliciousDelegates := 0

	for _, node := range dt.Node.Network.GetNodes() {
		// 判断恶意节点标准:
		// 1. 长期不投票的节点
		// 2. 出块失败率过高的代理节点
		if node.Type == types.VoterNode {
			if node.VotedFor == "" && node.IsActive {
				maliciousVoters++
			}
		} else if node.Type == types.DelegateNode {
			successRate := 0.0
			totalAttempts := node.BlocksProduced + node.MissedBlocks
			if totalAttempts > 0 {
				successRate = float64(node.BlocksProduced) / float64(totalAttempts)
			}
			// 成功率低于50%视为恶意
			if successRate < 0.5 && totalAttempts > 5 {
				maliciousDelegates++
			}
		}
	}

	data := MaliciousNodeData{
		Round:              round,
		Timestamp:          time.Now(),
		MaliciousVoters:    maliciousVoters,
		MaliciousDelegates: maliciousDelegates,
		TotalMalicious:     maliciousVoters + maliciousDelegates,
	}

	dt.MaliciousNodeData = append(dt.MaliciousNodeData, data)
}

// ================================
// 辅助函数
// ================================

func (dt *DPoSTester) getRecentBlocks(seconds int) []*network.Block {
	blocks := make([]*network.Block, 0)
	blockPool := dt.Node.BlockPool

	latestHeight := blockPool.GetLatestHeight()
	cutoffTime := time.Now().Add(-time.Duration(seconds) * time.Second)

	for i := latestHeight; i > 0; i-- {
		block, err := blockPool.GetBlock(i)
		if err != nil {
			break
		}
		if block.Timestamp.After(cutoffTime) {
			blocks = append(blocks, block)
		} else {
			break
		}
	}

	return blocks
}

func (dt *DPoSTester) getLatencyBucket(latency int64) string {
	switch {
	case latency <= 25:
		return "0-25ms"
	case latency <= 50:
		return "25-50ms"
	case latency <= 75:
		return "50-75ms"
	case latency <= 100:
		return "75-100ms"
	default:
		return ">100ms"
	}
}

// ================================
// 报告生成
// ================================

func (dt *DPoSTester) generateAllReports() {
	dt.Logger.Info("生成测试报告...")

	// 保存CSV数据
	dt.saveThroughputReport()
	dt.saveLatencyReport()
	dt.saveBlockProductionReport()
	dt.saveVotingSpeedReport()
	dt.saveMaliciousNodeReport()

	// 生成综合分析
	dt.generateSummaryReport()

	dt.Logger.Info("✓ 所有报告已生成!")
}

func (dt *DPoSTester) generateBasicReports() {
	dt.saveThroughputReport()
	dt.saveLatencyReport()
	dt.generateSummaryReport()
	dt.Logger.Info("✓ 基础报告已生成!")
}

// 保存吞吐量报告
func (dt *DPoSTester) saveThroughputReport() {
	filename := filepath.Join(dt.OutputDir, "吞吐量数据.csv")
	file, err := os.Create(filename)
	if err != nil {
		dt.Logger.Error("创建吞吐量报告失败: %v", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"轮次", "时间戳", "节点数量", "TPS", "每分钟区块数", "平均出块时间(秒)"}
	writer.Write(header)

	for _, data := range dt.ThroughputData {
		row := []string{
			fmt.Sprintf("%d", data.Round),
			data.Timestamp.Format(time.RFC3339),
			fmt.Sprintf("%d", data.TotalNodes),
			fmt.Sprintf("%.2f", data.TPS),
			fmt.Sprintf("%.2f", data.BlocksPerMinute),
			fmt.Sprintf("%.2f", data.AvgBlockTime),
		}
		writer.Write(row)
	}

	dt.Logger.Info("✓ 吞吐量报告: %s", filename)
}

// 保存时延报告
func (dt *DPoSTester) saveLatencyReport() {
	filename := filepath.Join(dt.OutputDir, "时延数据.csv")
	file, err := os.Create(filename)
	if err != nil {
		dt.Logger.Error("创建时延报告失败: %v", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{
		"轮次",
		"时间戳",
		"节点数量",
		"总共识时间(ms)",  // 投票+出块
		"投票阶段时间(ms)", // 真实投票耗时
		"出块时间(ms)",   // (已包含在共识时间内)
		"区块传播时间(ms)", // 广播耗时
	}
	writer.Write(header)

	for _, data := range dt.LatencyData {
		row := []string{
			fmt.Sprintf("%d", data.Round),
			data.Timestamp.Format(time.RFC3339),
			fmt.Sprintf("%d", data.TotalNodes),
			fmt.Sprintf("%.2f", data.AvgConsensusTime),
			fmt.Sprintf("%.2f", data.AvgVotingTime),
			fmt.Sprintf("%.2f", data.AvgConsensusTime-data.AvgVotingTime), // 出块时间
			fmt.Sprintf("%.2f", data.AvgBlockPropTime),
		}
		writer.Write(row)
	}

	dt.Logger.Info("✓ 时延报告: %s", filename)
}

// 保存区块生产占比报告
func (dt *DPoSTester) saveBlockProductionReport() {
	filename := filepath.Join(dt.OutputDir, "区块生产占比.csv")
	file, err := os.Create(filename)
	if err != nil {
		dt.Logger.Error("创建区块生产报告失败: %v", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"轮次", "时间戳", "节点ID", "节点排名", "生产区块数", "总区块数", "生产占比"}
	writer.Write(header)

	for _, data := range dt.BlockProductionData {
		row := []string{
			fmt.Sprintf("%d", data.Round),
			data.Timestamp.Format(time.RFC3339),
			data.NodeID,
			fmt.Sprintf("%d", data.NodeRank),
			fmt.Sprintf("%d", data.BlocksProduced),
			fmt.Sprintf("%d", data.TotalBlocks),
			fmt.Sprintf("%.4f", data.ProductionRate),
		}
		writer.Write(row)
	}

	dt.Logger.Info("✓ 区块生产占比报告: %s", filename)
}

// 保存投票速率报告
func (dt *DPoSTester) saveVotingSpeedReport() {
	filename := filepath.Join(dt.OutputDir, "投票速率数据.csv")
	file, err := os.Create(filename)
	if err != nil {
		dt.Logger.Error("创建投票速率报告失败: %v", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"轮次", "时间戳", "节点ID", "投票延迟(ms)", "延迟区间"}
	writer.Write(header)

	for _, data := range dt.VotingSpeedData {
		row := []string{
			fmt.Sprintf("%d", data.Round),
			data.Timestamp.Format(time.RFC3339),
			data.NodeID,
			fmt.Sprintf("%d", data.VoteLatency),
			data.LatencyBucket,
		}
		writer.Write(row)
	}

	dt.Logger.Info("✓ 投票速率报告: %s", filename)
}

// 保存恶意节点报告
func (dt *DPoSTester) saveMaliciousNodeReport() {
	filename := filepath.Join(dt.OutputDir, "恶意节点数据.csv")
	file, err := os.Create(filename)
	if err != nil {
		dt.Logger.Error("创建恶意节点报告失败: %v", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"轮次", "时间戳", "恶意投票节点数", "恶意代理节点数", "总恶意节点数"}
	writer.Write(header)

	for _, data := range dt.MaliciousNodeData {
		row := []string{
			fmt.Sprintf("%d", data.Round),
			data.Timestamp.Format(time.RFC3339),
			fmt.Sprintf("%d", data.MaliciousVoters),
			fmt.Sprintf("%d", data.MaliciousDelegates),
			fmt.Sprintf("%d", data.TotalMalicious),
		}
		writer.Write(row)
	}

	dt.Logger.Info("✓ 恶意节点报告: %s", filename)
}

// 生成综合摘要报告
func (dt *DPoSTester) generateSummaryReport() {
	filename := filepath.Join(dt.OutputDir, "测试摘要.json")

	// 计算平均值
	avgTPS := 0.0
	avgLatency := 0.0
	for _, data := range dt.ThroughputData {
		avgTPS += data.TPS
	}
	for _, data := range dt.LatencyData {
		avgLatency += data.AvgConsensusTime
	}

	if len(dt.ThroughputData) > 0 {
		avgTPS /= float64(len(dt.ThroughputData))
	}
	if len(dt.LatencyData) > 0 {
		avgLatency /= float64(len(dt.LatencyData))
	}

	summary := map[string]interface{}{
		"测试信息": map[string]interface{}{
			"节点ID": dt.Node.LocalNode.ID,
			"测试轮数": dt.TestRounds,
			"测试时长": time.Since(dt.StartTime).String(),
			"算法类型": "DPoS (原始)",
		},
		"性能指标": map[string]interface{}{
			"平均TPS": fmt.Sprintf("%.2f", avgTPS),
			"平均时延":  fmt.Sprintf("%.2f ms", avgLatency),
		},
		"生成文件": []string{
			"吞吐量数据.csv",
			"时延数据.csv",
			"区块生产占比.csv",
			"投票速率数据.csv",
			"恶意节点数据.csv",
		},
	}

	data, _ := json.MarshalIndent(summary, "", "  ")
	os.WriteFile(filename, data, 0644)

	dt.Logger.Info("✓ 测试摘要: %s", filename)
}

// GetSummaryStats 获取摘要统计
func (dt *DPoSTester) GetSummaryStats() map[string]interface{} {
	if len(dt.RoundResults) == 0 {
		return map[string]interface{}{}
	}

	last := dt.RoundResults[len(dt.RoundResults)-1]

	return map[string]interface{}{
		"NodeID":         dt.Node.LocalNode.ID,
		"NodeType":       last.NodeType,
		"BlocksProduced": last.BlocksProduced,
		"TotalReward":    last.TotalReward,
		"IsActive":       last.IsActive,
	}
}
