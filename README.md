# croj-sandbox

croj-sandbox 是一个轻量级代码执行沙箱，用于安全地编译和运行多种编程语言的代码。它提供本地执行环境和API接口，可集成到各类在线评测系统中。

## 功能特点

- 多语言支持：支持Go、C++、Python、Java、JavaScript等编程语言
- 代码编译：在安全的临时环境中编译源代码
- 代码执行：运行编译后的程序并收集结果
- 输出比较：支持与预期输出进行比较（用于评测答案正确性）
- 限制控制：支持编译超时、执行超时、输出大小限制等
- 结果收集：包括标准输出、标准错误、退出码、执行时间等
- API接口：提供HTTP API接口，方便集成到其他系统

## 支持的编程语言

- Go
- C++
- Python
- Java
- JavaScript (Node.js)

## 支持的评测结果

- Accepted：代码成功编译并正确执行
- Wrong Answer：代码执行输出与预期结果不匹配
- Compile Error：代码编译失败
- Runtime Error：运行时错误（如除零、非零退出码等）
- Time Limit Exceeded：执行超时
- Output Limit Exceeded：输出超过最大限制
- Sandbox Error：沙箱内部错误

## 使用方法

### 命令行直接运行

```bash
# 运行Go语言测试用例
go run cmd/croj-sandbox/main.go -lang go

# 运行C++测试用例
go run cmd/croj-sandbox/main.go -lang cpp

# 自定义执行时间限制
go run cmd/croj-sandbox/main.go -lang python -time 5
```

### 使用客户端工具

```bash
# 编译客户端工具
go build -o simple-client cmd/simple-client/main.go

# 本地执行Go代码文件
./simple-client -source main.go

# 提供标准输入
./simple-client -source main.go -stdin input.txt

# 与预期输出比较
./simple-client -source main.go -output expected.txt

# 指定编程语言（不使用扩展名推断）
./simple-client -source code.txt -lang python

# 向远程API发送执行请求
./simple-client -source main.go -api http://localhost:8080/execute
```

### 启动API服务器

```bash
# 编译API服务器
go build -o api-server cmd/api-server/main.go

# 启动API服务器
./api-server

# 自定义端口
./api-server -port 9000

# 自定义临时目录
./api-server -temp-dir /tmp/sandbox-temp
```

### 作为库使用

```go
import "github.com/CodeRushOJ/croj-sandbox/internal/sandbox"

func main() {
    // 创建默认配置
    cfg := sandbox.DefaultConfig()
    
    // 初始化沙箱运行器
    runner, err := sandbox.NewRunner(cfg)
    if err != nil {
        log.Fatalf("初始化沙箱失败: %v", err)
    }
    defer runner.Close()
    
    // 运行Go代码
    code := `
    package main
    import "fmt"
    func main() {
        fmt.Println("Hello, Sandbox!")
    }
    `
    
    result := runner.Run(context.Background(), "go", code, nil, nil)
    
    // 处理结果
    fmt.Printf("状态: %s\n", result.Status)
    fmt.Printf("输出: %s\n", result.Stdout)
}
```

## 配置参数

- DefaultCompileTimeLimit: 默认编译超时时间（默认10秒）
- DefaultExecuteTimeLimit: 默认执行超时时间（默认3秒）
- DefaultExecuteMemoryLimit: 默认内存限制（默认512MB）
- MaxStdoutSize: 标准输出最大字节数（默认64KB）
- MaxStderrSize: 标准错误最大字节数（默认64KB）
- HostTempDir: 临时文件目录（默认/tmp/croj-sandbox-local-runs）

## 未来计划

- 支持内存限制检测
- Docker容器隔离支持
- 更多编程语言支持