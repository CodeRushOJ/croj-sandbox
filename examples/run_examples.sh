#!/bin/bash

# 运行目录中的所有示例，测试输入/输出比较功能

# 确保 simple-client 已经编译
cd $(dirname "$0")/..
if [ ! -f "./simple-client" ]; then
  echo "编译 simple-client..."
  go build -o simple-client cmd/simple-client/main.go
fi

# 颜色输出函数
function print_header() {
  echo -e "\033[1;34m============== $1 ==============\033[0m"
}

function print_success() {
  echo -e "\033[1;32m✓ $1\033[0m"
}

function print_fail() {
  echo -e "\033[1;31m✗ $1\033[0m"
}

# Go 语言示例
print_header "运行 Go 示例"
./simple-client -source examples/go/main.go -stdin examples/go/sum_input.txt -output examples/go/sum_output.txt

# C++ 语言示例
print_header "运行 C++ 示例"
./simple-client -source examples/cpp/main.cpp -stdin examples/cpp/sort_input.txt -output examples/cpp/sort_output.txt

# Python 语言示例
print_header "运行 Python 示例"
./simple-client -source examples/python/main.py -stdin examples/python/calculator_input.txt -output examples/python/calculator_output.txt

# Java 语言示例
print_header "运行 Java 示例"
./simple-client -source examples/java/Main.java -stdin examples/java/wordcount_input.txt -output examples/java/wordcount_output.txt

# JavaScript 语言示例
print_header "运行 JavaScript 示例"
./simple-client -source examples/javascript/main.js -stdin examples/javascript/fibonacci_input.txt -output examples/javascript/fibonacci_output.txt

echo "所有示例测试完成！"
