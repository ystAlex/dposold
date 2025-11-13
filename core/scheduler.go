package core

import (
	"dpos/types"
	"sync"
)

// BlockScheduler 出块调度器
// 负责管理代理节点的出块时隙分配
// 基于确定性算法，保证所有节点对出块顺序达成共识
type BlockScheduler struct {
	mu sync.RWMutex

	// 当前代理节点列表(按选举结果排序)
	Delegates []*types.Node

	// 当前时隙索引(0 到 len(Delegates)-1)
	CurrentSlot int

	// 当前区块高度
	CurrentHeight int
}

// NewBlockScheduler 创建调度器实例
func NewBlockScheduler() *BlockScheduler {
	return &BlockScheduler{
		Delegates:     make([]*types.Node, 0),
		CurrentSlot:   0,
		CurrentHeight: 0,
	}
}

// UpdateDelegates 更新代理节点列表
// 在每轮选举后调用，重置时隙计数器
// 参数:
//   - delegates: 新选举出的代理节点列表(已排序)
func (bs *BlockScheduler) UpdateDelegates(delegates []*types.Node) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	bs.Delegates = delegates
	bs.CurrentSlot = 0 // 重置时隙索引
}

// GetCurrentProducer 获取当前时隙的出块者
// 基于时隙索引循环分配
// 返回:
//   - *types.Node: 当前应该出块的节点
//   - bool: 是否成功获取(false表示代理列表为空)
func (bs *BlockScheduler) GetCurrentProducer() (*types.Node, bool) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	if len(bs.Delegates) == 0 {
		return nil, false
	}

	// 循环分配时隙
	slotIndex := bs.CurrentSlot % len(bs.Delegates)
	producer := bs.Delegates[slotIndex]

	return producer, true
}

// NextSlot 前进到下一个时隙
// 在每个区块生产完成后调用
func (bs *BlockScheduler) NextSlot() {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	bs.CurrentSlot++

	// 一轮结束，重置时隙索引
	if bs.CurrentSlot >= len(bs.Delegates) {
		bs.CurrentSlot = 0
	}
}

// ShouldProduce 判断指定节点是否应该在当前时隙出块
// 参数:
//   - nodeID: 要检查的节点ID
//
// 返回: true表示该节点是当前出块者
func (bs *BlockScheduler) ShouldProduce(nodeID string) bool {
	producer, ok := bs.GetCurrentProducer()
	if !ok {
		return false
	}

	return producer.ID == nodeID
}

// GetSlotInfo 获取当前时隙信息
// 用于调试和状态查询
func (bs *BlockScheduler) GetSlotInfo() map[string]interface{} {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	producer, _ := bs.GetCurrentProducer()
	producerID := ""
	if producer != nil {
		producerID = producer.ID
	}

	roundProgress := 0.0
	if len(bs.Delegates) > 0 {
		roundProgress = float64(bs.CurrentSlot) / float64(len(bs.Delegates))
	}

	return map[string]interface{}{
		"current_slot":     bs.CurrentSlot,
		"current_height":   bs.CurrentHeight,
		"total_delegates":  len(bs.Delegates),
		"current_producer": producerID,
		"round_progress":   roundProgress,
	}
}

// IsReady 检查调度器是否已初始化
// 返回false表示还没有代理节点
func (bs *BlockScheduler) IsReady() bool {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return len(bs.Delegates) > 0
}

// GetDelegateCount 获取代理节点数量
func (bs *BlockScheduler) GetDelegateCount() int {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	return len(bs.Delegates)
}

// GetDelegateBySlot 根据时隙索引获取代理节点
// 参数:
//   - slot: 时隙索引
//
// 返回: 对应的代理节点(nil表示索引越界)
func (bs *BlockScheduler) GetDelegateBySlot(slot int) *types.Node {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	if slot < 0 || slot >= len(bs.Delegates) {
		return nil
	}

	return bs.Delegates[slot]
}

// Reset 重置调度器状态
// 用于测试或重新初始化
func (bs *BlockScheduler) Reset() {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	bs.Delegates = make([]*types.Node, 0)
	bs.CurrentSlot = 0
	bs.CurrentHeight = 0
}
