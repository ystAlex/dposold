#!/bin/bash

set -e

# ============================================
# DPoS 多节点网络启动脚本
# ============================================

# 配置参数
NUM_NODES=5              # 节点数量
BASE_PORT=13000          # 起始端口
TEST_ROUNDS=20          # 测试轮数
OUTPUT_DIR="./test_results"  # 输出目录

# 颜色输出
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}DPoS 多节点网络启动脚本${NC}"
echo -e "${GREEN}========================================${NC}"
echo -e "${BLUE}节点数量: ${NUM_NODES}${NC}"
echo -e "${BLUE}起始端口: ${BASE_PORT}${NC}"
echo -e "${BLUE}测试轮数: ${TEST_ROUNDS}${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# ============================================
# 计算统一的创世时间（当前时间+1分钟）
# ============================================
GENESIS_TIME=$(date -u -d '+1 minute' '+%Y-%m-%dT%H:%M:%S+00:00' 2>/dev/null || date -u -v+1M '+%Y-%m-%dT%H:%M:%S+00:00')

echo -e "${GREEN}创世时间: ${GENESIS_TIME}${NC}"
echo -e "${YELLOW}所有节点将在此时间同步启动区块产生${NC}"
echo ""

# ============================================
# 1. 清理环境
# ============================================
echo -e "${YELLOW}[1/6] 清理旧进程和日志...${NC}"
pkill -f "bin/dpos" || true
sleep 2

rm -rf logs
rm -rf ${OUTPUT_DIR}
mkdir -p logs
mkdir -p ${OUTPUT_DIR}

echo -e "${GREEN}✓ 清理完成${NC}"
echo ""

# ============================================
# 2. 编译程序
# ============================================
echo -e "${YELLOW}[2/6] 编译程序...${NC}"

if [ ! -f "../cmd/seed/main.go" ]; then
    echo -e "${RED}✗ 错误: 找不到 main.go${NC}"
    echo -e "${YELLOW}请确保在项目根目录运行脚本${NC}"
    exit 1
fi

go build -o bin/dpos ../cmd/seed/main.go

if [ $? -ne 0 ]; then
    echo -e "${RED}✗ 编译失败${NC}"
    exit 1
fi

echo -e "${GREEN}✓ 编译成功${NC}"
echo ""

# ============================================
# 健康检查函数
# ============================================
check_node_ready() {
    local addr=$1
    local max_attempts=30

    for i in $(seq 1 $max_attempts); do
        if curl -s "http://${addr}/status" > /dev/null 2>&1; then
            return 0
        fi
        sleep 1
    done

    return 1
}

# ============================================
# 3. 启动种子节点
# ============================================
echo -e "${YELLOW}[3/6] 启动种子节点 (node-0)${NC}"
echo -e "${GREEN}========================================${NC}"

SEED_ADDR="localhost:${BASE_PORT}"

echo "  节点ID: node-0"
echo "  地址: ${SEED_ADDR}"
echo "  质押: 1000"
echo "  创世时间: ${GENESIS_TIME}"

./bin/dpos \
    -id="node-0" \
    -listen="${SEED_ADDR}" \
    -stake=1000 \
    -delay=10 \
    -genesis="${GENESIS_TIME}" \
    -test \
    -rounds=${TEST_ROUNDS} \
    -output="${OUTPUT_DIR}" \
    -log=2 \
    > logs/node-0.log 2>&1 &

SEED_PID=$!
echo "  PID: ${SEED_PID}"

echo -e "${YELLOW}等待种子节点启动...${NC}"
if check_node_ready "${SEED_ADDR}"; then
    echo -e "${GREEN}✓ 种子节点已就绪${NC}"
else
    echo -e "${RED}✗ 种子节点启动超时${NC}"
    exit 1
fi

echo ""

# ============================================
# 4. 启动其他节点
# ============================================
echo -e "${YELLOW}[4/6] 启动其他节点${NC}"
echo -e "${GREEN}========================================${NC}"

for i in $(seq 1 $((NUM_NODES-1))); do
    PORT=$((BASE_PORT + i))
    ADDR="localhost:${PORT}"

    # 使用case语句定义不同节点的配置（避免关联数组）
    case $i in
        1) STAKE=800; DELAY=15 ;;
        2) STAKE=600; DELAY=20 ;;
        3) STAKE=400; DELAY=25 ;;
        4) STAKE=200; DELAY=30 ;;
        *) STAKE=500; DELAY=20 ;;  # 默认配置
    esac

    echo ""
    echo -e "${BLUE}启动节点 node-${i}${NC}"
    echo "  地址: ${ADDR}"
    echo "  种子: ${SEED_ADDR}"
    echo "  质押: ${STAKE}"
    echo "  延迟: ${DELAY}ms"
    echo "  创世时间: ${GENESIS_TIME}"

    ./bin/dpos \
        -id="node-${i}" \
        -listen="${ADDR}" \
        -seeds="${SEED_ADDR}" \
        -stake=${STAKE} \
        -delay=${DELAY} \
        -genesis="${GENESIS_TIME}" \
        -test \
        -rounds=${TEST_ROUNDS} \
        -output="${OUTPUT_DIR}" \
        -log=2 \
        > logs/node-${i}.log 2>&1 &

    NODE_PID=$!
    echo "  PID: ${NODE_PID}"

    echo -e "${YELLOW}等待节点 node-${i} 启动...${NC}"
    if check_node_ready "${ADDR}"; then
        echo -e "${GREEN}✓ 节点 node-${i} 已就绪${NC}"
    else
        echo -e "${RED}✗ 节点 node-${i} 启动超时${NC}"
        echo -e "${YELLOW}查看日志: tail logs/node-${i}.log${NC}"
    fi

    sleep 3  # 给节点间发现留出时间
done

echo ""
echo -e "${GREEN}✓ 所有节点已启动${NC}"
echo ""

# ============================================
# 5. 验证节点连接状态
# ============================================
echo -e "${YELLOW}[5/6] 验证节点连接状态${NC}"
echo -e "${GREEN}========================================${NC}"

sleep 10  # 等待节点发现完成

echo ""
printf "%-10s %-15s %-10s %-10s\n" "节点" "地址" "对等节点" "状态"
echo "--------------------------------------------------------"

for i in $(seq 0 $((NUM_NODES-1))); do
    PORT=$((BASE_PORT + i))
    ADDR="localhost:${PORT}"

    # 获取状态
    STATUS=$(curl -s "http://${ADDR}/status" 2>/dev/null || echo "{}")
    PEER_COUNT=$(echo $STATUS | grep -o '"connected_peers":[0-9]*' | cut -d: -f2)
    IS_RUNNING=$(echo $STATUS | grep -o '"is_running":[a-z]*' | cut -d: -f2)

    if [ -z "$PEER_COUNT" ]; then
        PEER_COUNT=0
    fi

    # 状态着色
    if [ "$IS_RUNNING" = "true" ] && [ "$PEER_COUNT" -gt 0 ]; then
        STATUS_TEXT="${GREEN}正常${NC}"
    else
        STATUS_TEXT="${RED}异常${NC}"
    fi

    printf "%-10s %-15s %-10s " "node-${i}" "${ADDR}" "${PEER_COUNT}"
    echo -e "${STATUS_TEXT}"
done

echo ""

# ============================================
# 6. 等待创世时间倒计时
# ============================================
echo -e "${YELLOW}[6/6] 等待创世时间到达...${NC}"
echo -e "${GREEN}========================================${NC}"

GENESIS_TIMESTAMP=$(date -d "${GENESIS_TIME}" +%s 2>/dev/null || date -j -f "%Y-%m-%dT%H:%M:%S+00:00" "${GENESIS_TIME}" +%s)

echo ""
while true; do
    NOW=$(date +%s)
    REMAINING=$((GENESIS_TIMESTAMP - NOW))

    if [ $REMAINING -le 0 ]; then
        echo -e "\n${GREEN}✓ 创世时间已到达！区块产生已开始${NC}"
        break
    fi

    # 计算时分秒
    MINS=$((REMAINING / 60))
    SECS=$((REMAINING % 60))

    echo -ne "\r${YELLOW}⏰ 倒计时: ${MINS}分${SECS}秒...${NC}"
    sleep 1
done

echo ""
echo ""

# ============================================
# 启动完成提示
# ============================================
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}🎉 网络启动完成！${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "${BLUE}网络信息：${NC}"
echo "  节点数量: ${NUM_NODES}"
echo "  创世时间: ${GENESIS_TIME}"
echo "  测试轮数: ${TEST_ROUNDS}"
echo "  区块间隔: 3秒"
echo ""
echo -e "${BLUE}监控命令：${NC}"
echo "  查看节点0日志: tail -f logs/node-0.log"
echo "  查看所有日志: ls -lh logs/"
echo "  查看节点状态: curl http://localhost:12000/status"
echo ""
echo -e "${BLUE}测试结果：${NC}"
echo "  输出目录: ${OUTPUT_DIR}"
echo "  CSV数据: ${OUTPUT_DIR}/dpos_test_results.csv"
echo "  统计报告: ${OUTPUT_DIR}/statistics_report.txt"
echo ""
echo -e "${YELLOW}按 Ctrl+C 停止网络${NC}"
echo ""

# ============================================
# 实时监控函数
# ============================================
monitor_network() {
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}实时网络监控${NC}"
    echo -e "${GREEN}========================================${NC}"
    echo ""

    while true; do
        clear
        echo -e "${GREEN}DPoS 网络实时状态${NC}"
        echo -e "${BLUE}时间: $(date '+%Y-%m-%d %H:%M:%S')${NC}"
        echo "========================================"
        echo ""

        printf "%-10s %-10s %-15s %-15s %-10s\n" "节点" "类型" "质押量" "总奖励" "出块数"
        echo "------------------------------------------------------------------------"

        for i in $(seq 0 $((NUM_NODES-1))); do
            PORT=$((BASE_PORT + i))
            STATUS=$(curl -s "http://localhost:${PORT}/status" 2>/dev/null || echo "{}")

            NODE_TYPE=$(echo $STATUS | grep -o '"type":"[^"]*"' | cut -d'"' -f4)
            STAKE=$(echo $STATUS | grep -o '"stake":[0-9.]*' | cut -d: -f2)
            REWARD=$(echo $STATUS | grep -o '"total_reward":[0-9.]*' | cut -d: -f2)
            BLOCKS=$(echo $STATUS | grep -o '"blocks_produced":[0-9]*' | cut -d: -f2)

            # 默认值
            NODE_TYPE=${NODE_TYPE:-"Voter"}
            STAKE=${STAKE:-"0"}
            REWARD=${REWARD:-"0"}
            BLOCKS=${BLOCKS:-"0"}

            # 类型着色
            if [ "$NODE_TYPE" = "Delegate" ]; then
                TYPE_COLOR="${GREEN}"
            else
                TYPE_COLOR="${YELLOW}"
            fi

            printf "%-10s ${TYPE_COLOR}%-10s${NC} %-15.2f %-15.2f %-10s\n" \
                "node-${i}" "$NODE_TYPE" "$STAKE" "$REWARD" "$BLOCKS"
        done

        echo ""
        echo "========================================"
        echo -e "${YELLOW}按 Ctrl+C 停止监控${NC}"

        sleep 5
    done
}

# ============================================
# 等待中断或启动监控
# ============================================
echo -e "${BLUE}选择操作：${NC}"
echo "  1. 后台运行（日志查看）"
echo "  2. 实时监控面板"
echo ""
read -p "请选择 [1/2]: " CHOICE

case $CHOICE in
    2)
        monitor_network
        ;;
    *)
        echo -e "${GREEN}网络在后台运行中...${NC}"
        echo ""
        ;;
esac

# 捕获中断信号
trap 'echo -e "\n${RED}停止所有节点...${NC}"; pkill -f "bin/dpos"; echo -e "${GREEN}✓ 网络已停止${NC}"; exit 0' INT TERM

wait
