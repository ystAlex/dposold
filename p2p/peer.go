package p2p

import (
	"sync"
	"time"
)

// ================================
// 对等节点管理
// ================================

// PeerInfo 对等节点信息结构体
// 存储单个对等节点的详细信息
type PeerInfo struct {
	NodeID      string    // 节点唯一标识符
	Address     string    // 网络地址(格式: IP:Port)
	PublicKey   string    // 节点公钥(用于验证签名)
	LastSeen    time.Time // 最后在线时间
	IsActive    bool      // 是否活跃
	Latency     float64   // 网络延迟(毫秒)
	IsDelegate  bool      // 是否为代理节点
	Weight      float64   // 节点权重(质押量)
	Performance float64   // 表现评分(0-1)
}

// PeerManager 对等节点管理器
// 负责维护和管理所有已知的对等节点
type PeerManager struct {
	mu          sync.RWMutex         // 读写锁(保护并发访问)
	peers       map[string]*PeerInfo // 对等节点映射(key: nodeID)
	localNodeID string               // 本地节点ID
	maxPeers    int                  // 最大对等节点数
}

// NewPeerManager 创建对等节点管理器实例
// 参数:
//   - localNodeID: 本地节点ID
//   - maxPeers: 最大对等节点数量限制
func NewPeerManager(localNodeID string, maxPeers int) *PeerManager {
	return &PeerManager{
		peers:       make(map[string]*PeerInfo),
		localNodeID: localNodeID,
		maxPeers:    maxPeers,
	}
}

// AddPeer 添加对等节点
// 参数:
//   - peer: 对等节点信息
//
// 如果节点已存在，更新其信息
func (pm *PeerManager) AddPeer(peer *PeerInfo) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// 直接添加或更新节点信息
	pm.peers[peer.NodeID] = peer
	return nil
}

// RemovePeer 移除对等节点
// 参数:
//   - nodeID: 要移除的节点ID
func (pm *PeerManager) RemovePeer(nodeID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.peers, nodeID)
}

// GetPeer 获取指定节点的信息
// 参数:
//   - nodeID: 节点ID
//
// 返回:
//   - *PeerInfo: 节点信息
//   - bool: 是否找到
func (pm *PeerManager) GetPeer(nodeID string) (*PeerInfo, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	peer, exists := pm.peers[nodeID]
	return peer, exists
}

// GetAllPeers 获取所有对等节点
// 返回所有已知节点的副本列表
func (pm *PeerManager) GetAllPeers() []*PeerInfo {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	peers := make([]*PeerInfo, 0, len(pm.peers))
	for _, peer := range pm.peers {
		peers = append(peers, peer)
	}
	return peers
}

// GetActivePeers 获取活跃对等节点
// 返回所有IsActive=true的节点
func (pm *PeerManager) GetActivePeers() []*PeerInfo {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	peers := make([]*PeerInfo, 0)
	for _, peer := range pm.peers {
		if peer.IsActive {
			peers = append(peers, peer)
		}
	}
	return peers
}

// UpdatePeerStatus 更新对等节点状态
// 参数:
//   - nodeID: 节点ID
//   - isActive: 是否活跃
//   - weight: 节点权重
//   - performance: 性能评分
func (pm *PeerManager) UpdatePeerStatus(nodeID string, isActive bool, weight, performance float64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if peer, exists := pm.peers[nodeID]; exists {
		peer.IsActive = isActive
		peer.Weight = weight
		peer.Performance = performance
		peer.LastSeen = time.Now()
	}
}

// GetDelegates 获取代理节点列表
// 返回所有IsDelegate=true且活跃的节点
func (pm *PeerManager) GetDelegates() []*PeerInfo {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	delegates := make([]*PeerInfo, 0)
	for _, peer := range pm.peers {
		if peer.IsDelegate && peer.IsActive {
			delegates = append(delegates, peer)
		}
	}
	return delegates
}

// PrunePeers 清理不活跃的节点
// 参数:
//   - timeout: 超时时间(超过此时间未见的节点将被移除)
//
// 返回清理的节点数量
func (pm *PeerManager) PrunePeers(timeout time.Duration) int {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	now := time.Now()
	pruned := 0

	// 遍历所有节点，移除超时的
	for nodeID, peer := range pm.peers {
		if now.Sub(peer.LastSeen) > timeout {
			delete(pm.peers, nodeID)
			pruned++
		}
	}

	return pruned
}

// GetPeerCount 获取节点总数
func (pm *PeerManager) GetPeerCount() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.peers)
}

// IsPeerConnected 检查指定节点是否已连接
func (pm *PeerManager) IsPeerConnected(nodeID string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	_, exists := pm.peers[nodeID]
	return exists
}

// UpdatePeerLatency 更新节点延迟
// 参数:
//   - nodeID: 节点ID
//   - latency: 延迟时间(毫秒)
func (pm *PeerManager) UpdatePeerLatency(nodeID string, latency float64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if peer, exists := pm.peers[nodeID]; exists {
		peer.Latency = latency
		peer.LastSeen = time.Now()
	}
}
