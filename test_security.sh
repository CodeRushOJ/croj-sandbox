#!/bin/bash

# 测试安全限制功能

# 编译客户端
go build -o simple-client cmd/simple-client/main.go

# 测试系统调用限制
echo "测试系统调用限制："
cat > /tmp/test_syscall.go <<EOF
package main

import (
    "fmt"
    "os/exec"
)

func main() {
    // 尝试执行系统命令 - 这应该被seccomp阻止
    cmd := exec.Command("ls", "-la")
    out, err := cmd.Output()
    if err != nil {
        fmt.Println("预期的错误发生：", err)
    } else {
        fmt.Println("命令执行成功：", string(out))
    }
}
EOF

./simple-client -source /tmp/test_syscall.go -timeout 5 -v

# 测试网络访问限制
echo -e "\n测试网络访问限制："
cat > /tmp/test_network.py <<EOF
import socket
try:
    # 尝试建立网络连接
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.connect(("www.google.com", 80))
    print("连接成功（这不应该发生）")
    s.close()
except Exception as e:
    print(f"预期的错误发生: {e}")
EOF

./simple-client -source /tmp/test_network.py -lang python -timeout 5 -v

# 测试内存限制
echo -e "\n测试内存限制："
./simple-client -source examples/memory_test/main.cpp -mem 50 -v

echo "安全测试完成！"
