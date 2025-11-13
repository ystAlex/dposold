package network

import (
	"fmt"
	"sync"
	"time"
)

// ================================
// 交易相关数据结构
// ================================

// Transaction 交易结构体
// 表示一笔链上交易
type Transaction struct {
	ID        string    // 交易唯一标识符
	From      string    // 发送方地址
	To        string    // 接收方地址
	Amount    float64   // 转账金额
	Fee       float64   // 交易手续费
	Timestamp time.Time // 交易时间戳
	Signature []byte    // 交易签名(用于验证)
	Nonce     int64     // 随机数(防止重放攻击)
}

// TransactionPool 交易池
// 管理待打包和已确认的交易
type TransactionPool struct {
	mu           sync.RWMutex
	transactions map[string]*Transaction // 所有交易(key: 交易ID)
	pending      []*Transaction          // 待打包交易队列
	confirmed    map[string]bool         // 已确认交易集合
}

// NewTransactionPool 创建交易池实例
func NewTransactionPool() *TransactionPool {
	return &TransactionPool{
		transactions: make(map[string]*Transaction),
		pending:      make([]*Transaction, 0),
		confirmed:    make(map[string]bool),
	}
}

// AddTransaction 添加交易到池中
// 返回错误如果交易已存在或已确认
func (tp *TransactionPool) AddTransaction(tx *Transaction) error {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	// 检查交易是否已存在
	if _, exists := tp.transactions[tx.ID]; exists {
		return fmt.Errorf("交易 %s 已存在", tx.ID)
	}

	// 检查交易是否已确认
	if tp.confirmed[tx.ID] {
		return fmt.Errorf("交易 %s 已确认", tx.ID)
	}

	// 添加到交易池和待处理队列
	tp.transactions[tx.ID] = tx
	tp.pending = append(tp.pending, tx)

	return nil
}

// GetPendingTransactions 获取待打包的交易
// 参数:
//   - limit: 最多返回的交易数量
//
// 返回前N笔待处理交易
func (tp *TransactionPool) GetPendingTransactions(limit int) []*Transaction {
	tp.mu.RLock()
	defer tp.mu.RUnlock()

	if limit > len(tp.pending) {
		limit = len(tp.pending)
	}

	// 复制交易(避免外部修改)
	txs := make([]*Transaction, limit)
	copy(txs, tp.pending[:limit])

	return txs
}

// RemoveTransactions 移除已打包的交易
// 参数:
//   - txIDs: 要移除的交易ID列表
//
// 将交易标记为已确认并从待处理队列中移除
func (tp *TransactionPool) RemoveTransactions(txIDs []string) {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	// 构建ID集合用于快速查找
	idSet := make(map[string]bool)
	for _, id := range txIDs {
		idSet[id] = true
		tp.confirmed[id] = true     // 标记为已确认
		delete(tp.transactions, id) // 从交易池移除
	}

	// 从待处理队列中过滤掉已打包的交易
	newPending := make([]*Transaction, 0)
	for _, tx := range tp.pending {
		if !idSet[tx.ID] {
			newPending = append(newPending, tx)
		}
	}
	tp.pending = newPending
}

// GetPendingCount 获取待处理交易数量
func (tp *TransactionPool) GetPendingCount() int {
	tp.mu.RLock()
	defer tp.mu.RUnlock()
	return len(tp.pending)
}

// GetTransaction 获取指定ID的交易
// 返回:
//   - *Transaction: 交易对象
//   - bool: 是否找到
func (tp *TransactionPool) GetTransaction(txID string) (*Transaction, bool) {
	tp.mu.RLock()
	defer tp.mu.RUnlock()

	tx, exists := tp.transactions[txID]
	return tx, exists
}

// IsConfirmed 检查交易是否已确认
func (tp *TransactionPool) IsConfirmed(txID string) bool {
	tp.mu.RLock()
	defer tp.mu.RUnlock()
	return tp.confirmed[txID]
}

// ================================
// 区块相关数据结构
// ================================

// Block 区块结构体
// 表示链上的一个区块
type Block struct {
	Height       int            // 区块高度(从1开始)
	ProducerID   string         // 出块者节点ID
	Transactions []*Transaction // 包含的交易列表
	Timestamp    time.Time      // 区块生成时间
	PrevHash     string         // 前一个区块的哈希值
	Hash         string         // 当前区块哈希值
	Validators   []string       // 验证者列表
	Signature    []byte         // 区块签名
	StateRoot    string         // 状态树根哈希
}

// BlockPool 区块池
// 管理已生成的区块链
type BlockPool struct {
	mu           sync.RWMutex
	blocks       map[int]*Block    // 区块映射(key: 区块高度)
	latest       *Block            // 最新区块指针
	newestNumber int               // 最新区块号(用于同步)
	index        map[string]*Block // 哈希索引(key: 区块哈希)
}

// NewBlockPool 创建区块池实例
func NewBlockPool() *BlockPool {
	return &BlockPool{
		blocks: make(map[int]*Block),
		index:  make(map[string]*Block),
	}
}

// AddBlock 添加区块到区块池
// 返回错误如果:
// - 区块高度已存在
// - 区块高度不连续
// - 前一个区块哈希不匹配
func (bp *BlockPool) AddBlock(block *Block) error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	// 检查区块高度是否已存在
	if _, exists := bp.blocks[block.Height]; exists {
		return fmt.Errorf("区块高度 %d 已存在", block.Height)
	}

	// 验证区块高度连续性
	if bp.latest != nil && block.Height != bp.latest.Height+1 {
		return fmt.Errorf("区块高度不连续: 期望 %d, 实际 %d", bp.latest.Height+1, block.Height)
	}

	// 验证前一个区块哈希
	if bp.latest != nil && block.PrevHash != bp.latest.Hash {
		return fmt.Errorf("前一个区块哈希不匹配")
	}

	// 添加区块到各个索引
	bp.blocks[block.Height] = block
	bp.index[block.Hash] = block

	// 更新最新区块指针
	if bp.latest == nil || block.Height > bp.latest.Height {
		bp.latest = block
	}

	// 更新最新区块号
	if block.Height > bp.newestNumber {
		bp.newestNumber = block.Height
	}

	return nil
}

// GetBlock 根据高度获取区块
func (bp *BlockPool) GetBlock(height int) (*Block, error) {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	if block, exists := bp.blocks[height]; exists {
		return block, nil
	}

	return nil, fmt.Errorf("区块高度 %d 不存在", height)
}

// GetLatestBlock 获取最新区块
func (bp *BlockPool) GetLatestBlock() *Block {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.latest
}

// GetNewestBlockNumber 获取最新区块号
// 用于同步协议
func (bp *BlockPool) GetNewestBlockNumber() int {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.newestNumber
}

// SetNewestBlockNumber 更新最新区块号
// 参数:
//   - blockNumber: 新的区块号
//
// 仅在同步过程中使用
func (bp *BlockPool) SetNewestBlockNumber(blockNumber int) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	if blockNumber > bp.newestNumber {
		bp.newestNumber = blockNumber
	}
}

// GetBlockByHash 通过哈希获取区块
func (bp *BlockPool) GetBlockByHash(hash string) (*Block, error) {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	if block, exists := bp.index[hash]; exists {
		return block, nil
	}

	return nil, fmt.Errorf("区块哈希 %s 不存在", hash)
}

// GetLatestHeight 获取最新区块高度
func (bp *BlockPool) GetLatestHeight() int {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	if bp.latest == nil {
		return 0
	}
	return bp.latest.Height
}

// GetBlockRange 获取指定高度范围的区块
// 参数:
//   - startHeight: 起始高度
//   - endHeight: 结束高度
//
// 返回该范围内的所有区块
func (bp *BlockPool) GetBlockRange(startHeight, endHeight int) []*Block {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	blocks := make([]*Block, 0)
	for height := startHeight; height <= endHeight; height++ {
		if block, exists := bp.blocks[height]; exists {
			blocks = append(blocks, block)
		}
	}

	return blocks
}

// HasBlock 检查指定高度的区块是否存在
func (bp *BlockPool) HasBlock(height int) bool {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	_, exists := bp.blocks[height]
	return exists
}

// GetBlockCount 获取区块总数
func (bp *BlockPool) GetBlockCount() int {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return len(bp.blocks)
}
