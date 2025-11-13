使用说明
1. 单节点模式

基础启动：

go run main.go -id node-1 -listen localhost:8000 -seeds localhost:8001,localhost:8002


启动第一个节点（种子节点）：

go run main.go -id node-1 -listen localhost:8000 -stake 1000


启动第二个节点（连接到种子节点）：

go run main.go -id node-2 -listen localhost:8001 -stake 500 -seeds localhost:8000


启动第三个节点：

go run main.go -id node-3 -listen localhost:8002 -stake 300 -seeds localhost:8000,localhost:8001


带测试的启动：

go run main.go -id node-1 -listen localhost:8000 -test -rounds 50 -output ./results


自定义创世时间：

go run main.go -id node-1 -genesis "2024-01-01T00:00:00+08:00"

2. 批量测试模式

启动50个节点进行测试：

go run main.go -batch -nodes 50 -rounds 50 -output ./test_results

go run main.go -batch -nodes 100 -port 9000 -rounds 100


调整日志级别：

# ERROR级别（最少日志）
go run main.go -batch -nodes 50 -log 0

# DEBUG级别（最详细日志）
go run main.go -batch -nodes 50 -log 3

3. 完整参数说明
   参数	说明	默认值	示例
   -id	节点ID（必需）	无	node-1
   -listen	监听地址	localhost:8000	0.0.0.0:8000
   -seeds	种子节点地址	无	localhost:8001,localhost:8002
   -stake	质押代币数量	100.0	1000
   -delay	网络延迟(ms)	20.0	50
   -genesis	创世时间	当前时间+30s	2024-01-01T00:00:00+08:00
   -test	是否运行测试	false	-test
   -rounds	测试轮数	50	100
   -output	测试结果目录	./test_results	./results
   -batch	批量模式	false	-batch
   -nodes	批量模式节点数	50	100
   -port	起始端口	8000	9000
   -log	日志级别(0-3)	2	3