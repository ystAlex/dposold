#!/bin/bash

echo "停止所有 DPoS 节点..."
pkill -f "bin/dpos"

echo "清理日志..."
rm -rf logs/*

echo "✓ 网络已停止"
