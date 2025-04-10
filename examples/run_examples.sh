#!/bin/bash

# 运行目录中的所有示例，测试输入/输出比较功能

# 确保 simple-client 已经编译
cd $(dirname "$0")/..
if [ -f "./simple-client" ]; then
  rm -f simple-client
fi
echo "编译 simple-client..."
go build -o simple-client cmd/simple-client/main.go

# 颜色输出函数
function print_header() {
  echo "\033[1;34m============== $1 ==============\033[0m"
}

function print_success() {
  echo "\033[1;32m✓ $1\033[0m"
}

function print_fail() {
  echo "\033[1;31m✗ $1\033[0m"
}

# 内存限制参数（对于示例程序，128MB 足够了）
MEM_LIMIT=128

# 是否启用调试模式
DEBUG_FLAG=""
if [ "$1" == "-debug" ] || [ "$1" == "--debug" ]; then
  DEBUG_FLAG="-debug"
  echo "已启用调试模式，将显示详细日志"
fi

# Go 语言示例
print_header "运行 Go 示例"
./simple-client $DEBUG_FLAG -source examples/go/main.go -stdin examples/go/sum_input.txt -output examples/go/sum_output.txt -mem $MEM_LIMIT

# C++ 语言示例
print_header "运行 C++ 示例"
./simple-client $DEBUG_FLAG -source examples/cpp/main.cpp -stdin examples/cpp/sort_input.txt -output examples/cpp/sort_output.txt -mem $MEM_LIMIT

# Python 语言示例
print_header "运行 Python 示例"
./simple-client $DEBUG_FLAG -source examples/python/main.py -stdin examples/python/calculator_input.txt -output examples/python/calculator_output.txt -mem $MEM_LIMIT

# Java 语言示例（Java 需要更多内存）
print_header "运行 Java 示例"
./simple-client $DEBUG_FLAG -source examples/java/Main.java -stdin examples/java/wordcount_input.txt -output examples/java/wordcount_output.txt -mem 256

# JavaScript 语言示例
print_header "运行 JavaScript 示例"
./simple-client $DEBUG_FLAG -source examples/javascript/main.js -stdin examples/javascript/fibonacci_input.txt -output examples/javascript/fibonacci_output.txt -mem $MEM_LIMIT

# 添加明确的分隔线
echo "\033[1;33m=========================================\033[0m"
echo "\033[1;33m           资源限制测试                    \033[0m"
echo "\033[1;33m=========================================\033[0m"

# Memory test 示例
print_header "运行 Memory test 示例 (限制 $MEM_LIMIT MB)"
./simple-client $DEBUG_FLAG -source examples/memory_test/main.cpp -stdin examples/memory_test/input.txt -mem $MEM_LIMIT

# Time test 示例 - 明确设置为1秒超时
print_header "运行 Time test 示例 (限制 1 秒)"
./simple-client $DEBUG_FLAG -source examples/time_test/main.cpp -timeout 1 -mem $MEM_LIMIT

print_header "运行 Time test 示例 (限制 2 秒)"
./simple-client $DEBUG_FLAG -source examples/time_test/main.cpp -timeout 2 -mem $MEM_LIMIT

echo "所有示例测试完成！"
