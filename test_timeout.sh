#!/bin/bash

# 编译客户端
go build -o simple-client cmd/simple-client/main.go

# 设置颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}测试时间限制功能传递${NC}"
echo "==============================="

# 使用不同的超时值测试
for timeout in 1 2 3 5; do
  echo -e "${BLUE}测试 $timeout 秒超时:${NC}"
  ./simple-client -source examples/time_test/main.cpp -timeout $timeout -v
  
  echo "------------------------------"
  sleep 1
done
