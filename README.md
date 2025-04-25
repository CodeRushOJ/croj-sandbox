# croj-sandbox

croj-sandbox 是一个轻量级代码执行沙箱，用于安全地编译和运行多种编程语言的代码。它提供本地执行环境和API接口，可集成到各类在线评测系统中。

## 架构

croj-sandbox 包含一个核心的 gRPC API 服务器 (`api-server`)。该服务器负责接收代码执行请求，管理编译和执行过程，并返回结果。

为了实现服务发现和负载均衡，`api-server` 可以注册到 Zookeeper 集群中。客户端或其他服务可以通过 Zookeeper 发现可用的沙箱实例。

代码的实际执行发生在隔离的环境中，目前通过 Docker 容器实现，以提供更好的安全性和资源控制。

## 功能特点

- **多语言支持**：支持 Go、C++、Python、Java、JavaScript (Node.js) 等。
- **代码编译**：在安全环境中编译源代码。
- **代码执行**：在隔离的 Docker 容器中运行编译后的程序或解释型语言脚本。
- **资源限制**：通过 Docker 控制执行过程的 CPU、内存使用。
- **安全隔离**：利用 Docker 容器、Seccomp、Cgroups 等技术提供安全沙箱环境。
- **结果收集**：包括标准输出、标准错误、退出码、执行时间、内存使用等。
- **gRPC API**：提供 gRPC 接口 (`proto/sandbox.proto`)，方便集成。
- **服务发现**：支持通过 Zookeeper 进行服务注册与发现。

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

### 启动 gRPC API 服务器

```bash
# 编译 API 服务器
# (或者直接使用 Docker 镜像)
go build -o api-server ./cmd/api-server

# 启动 API 服务器 (默认监听 0.0.0.0:50051)
./api-server

# 指定 Zookeeper 地址进行服务注册
./api-server --zk=zookeeper1:2181,zookeeper2:2181

# 自定义监听地址
./api-server --addr :60000

# 自定义临时目录 (如果需要本地临时文件)
# ./api-server -temp-dir /tmp/sandbox-temp 
```

### 使用 Docker 运行

项目提供了 `Dockerfile` 用于构建和运行沙箱服务。

```bash
# 1. 构建 Docker 镜像
docker build -t croj-sandbox:latest .

# 2. 运行沙箱容器
#    -p 映射 gRPC 端口
#    --name 给容器命名
docker run -d --rm -p 50051:50051 --name sandbox-server croj-sandbox:latest

# 3. (可选) 连接到 Zookeeper 网络并注册
#    假设 Zookeeper 运行在名为 'zk_net' 的 Docker 网络中，服务名为 'zookeeper'
#    首先确保容器连接到该网络
docker network connect zk_net sandbox-server

#    然后启动容器时指定 Zookeeper 地址
docker run -d --rm -p 50051:50051 --network zk_net --name sandbox-server croj-sandbox:latest --zk=zookeeper:2181
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

## 依赖

- Go 1.21+
- Docker (用于构建和运行)
- Zookeeper (可选，用于服务发现)
- `libseccomp-dev` (构建时需要)
- `libseccomp2` (运行时需要)

Go 模块依赖请参见 `go.mod` 文件。

## 未来计划

- 完善资源限制的精细化控制和报告
- 支持更多编程语言和运行时环境
- 优化性能和安全性