package main

import (
	"dpos/node"
	"dpos/test"
	"dpos/utils"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var (
	// ================================
	// 节点配置
	// ================================
	nodeID     = flag.String("id", "", "节点ID(必需)")
	listenAddr = flag.String("listen", "localhost:8000", "监听地址")
	seedNodes  = flag.String("seeds", "", "种子节点地址列表(逗号分隔)")

	// 节点参数
	stake        = flag.Float64("stake", 100.0, "质押代币数量")
	networkDelay = flag.Float64("delay", 20.0, "网络延迟(ms)")

	// 时间配置
	genesisTime = flag.String("genesis", "", "创世时间(RFC3339格式,留空则自动计算)")

	// ================================
	// 测试配置
	// ================================
	runTest       = flag.Bool("test", false, "是否运行测试")
	testMode      = flag.String("mode", "full", "测试模式: full|basic")
	testRounds    = flag.Int("rounds", 20, "测试轮数")
	testOutputDir = flag.String("output", "./test_results", "测试结果输出目录")

	// ================================
	// 日志配置
	// ================================
	logLevel = flag.Int("log", 2, "日志级别: 0=ERROR, 1=WARN, 2=INFO, 3=DEBUG")
)

func main() {
	flag.Parse()

	// 验证必需参数
	if *nodeID == "" {
		fmt.Println("❌ 错误: 必须指定节点ID (-id)")
		flag.Usage()
		os.Exit(1)
	}

	// 创建日志器
	logger := utils.NewLoggerWithLevel(*nodeID, utils.LogLevel(*logLevel))

	logger.Info("========================================")
	logger.Info("DPoS 节点启动")
	logger.Info("========================================")
	logger.Info("节点ID: %s", *nodeID)
	logger.Info("监听地址: %s", *listenAddr)
	logger.Info("质押量: %.2f", *stake)
	logger.Info("网络延迟: %.2f ms", *networkDelay)

	// 解析种子节点
	var seeds []string
	if *seedNodes != "" {
		seeds = strings.Split(*seedNodes, ",")
		logger.Info("种子节点: %v", seeds)
	}

	// 解析创世时间
	var genesis time.Time
	var err error

	if *genesisTime != "" {
		genesis, err = time.Parse(time.RFC3339, *genesisTime)
		if err != nil {
			logger.Error("解析创世时间失败: %v", err)
			os.Exit(1)
		}
	} else {
		// 默认为当前时间+30秒
		genesis = time.Now().Add(30 * time.Second)
	}

	logger.Info("创世时间: %s", genesis.Format(time.RFC3339))
	logger.Info("========================================")

	// 创建P2P节点
	p2pNode := node.NewP2PNode(
		*nodeID,
		*stake,
		*networkDelay,
		*listenAddr,
		seeds,
		genesis,
		logger,
	)

	// 启动节点
	if err := p2pNode.Start(); err != nil {
		logger.Error("启动节点失败: %v", err)
		os.Exit(1)
	}

	logger.Info("✓ 节点启动成功")

	// ================================
	// 测试模式
	// ================================
	if *runTest {
		logger.Info("\n========================================")
		logger.Info("启动测试模式")
		logger.Info("测试模式: %s", *testMode)
		logger.Info("========================================")

		// 等待网络稳定
		logger.Info("等待网络稳定(30秒)...")
		time.Sleep(30 * time.Second)

		// 创建测试器
		tester := test.NewDPoSTester(p2pNode, logger, *testOutputDir)

		// 根据测试模式运行测试
		switch *testMode {
		case "full":
			// 完整测试 - 包含详细分析
			logger.Info("开始完整性能测试(%d轮)...", *testRounds)
			tester.RunFullTest(*testRounds)

		case "basic":
			// 基础测试 - 只记录基本数据
			logger.Info("开始基础测试(%d轮)...", *testRounds)
			tester.RunBasicTest(*testRounds)

		default:
			logger.Error("未知的测试模式: %s", *testMode)
			logger.Info("支持的模式: full(完整测试), basic(基础测试)")
		}

		logger.Info("\n========================================")
		logger.Info("✓ 测试完成!")
		logger.Info("结果已保存到: %s", *testOutputDir)
		logger.Info("========================================")
	}

	// ================================
	// 等待中断信号
	// ================================
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("\n节点正在运行，按 Ctrl+C 停止...")
	<-sigChan

	logger.Info("\n收到停止信号，正在关闭节点...")
	p2pNode.Stop()
	logger.Info("✓ 节点已停止")
}
