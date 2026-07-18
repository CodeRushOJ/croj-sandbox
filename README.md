# croj-sandbox

CodeRushOJ 的代码编译与执行节点。服务通过 gRPC 接收评测请求，在 Linux 上为每次执行创建独立临时目录，并结合超时、输出限制、进程监控、cgroup 与 seccomp 返回结构化结果。

> 当前状态：核心执行链路和 Kubernetes 原生服务发现接口已经具备；安全实现仍在持续加固。现阶段适合本地开发、CI 与受控集群验证，不应直接作为面向不可信公网代码的最终安全边界。

## 核心能力

- gRPC `SandboxService.Execute` 执行协议
- Go、C++、Python、Java、JavaScript 五种语言运行配置
- 编译超时、执行超时、内存监控与 64 KiB 标准输出/错误限制
- `Accepted`、`Wrong Answer`、`Compile Error`、`Runtime Error`、`Time Limit Exceeded`、`Memory Limit Exceeded`、`Output Limit Exceeded`、`Sandbox Error` 等结果
- 标准 gRPC Health Checking Protocol 和 gRPC reflection
- Kubernetes `Service`/`EndpointSlice` 原生发现，不依赖 ZooKeeper
- Linux cgroup v1/v2 与 seccomp 基础防护

## 系统位置

```text
croj-backend --RocketMQ--> croj-judging-server
                                   |
                                   | Kubernetes EndpointSlice + round-robin
                                   v
                         croj-sandbox Service :50051
                                   |
                                   v
                          sandbox Pods (Execute)
```

调度器只访问名为 `grpc` 的 Service 端口，并从 EndpointSlice 选择 `Ready=true`、`Terminating!=true` 的地址。沙箱实例无需自行注册；Pod readiness 状态就是服务发现事实源。

## 目录结构

```text
cmd/api-server/       gRPC 服务入口和健康检查
internal/sandbox/     语言配置、编译、执行与结果模型
internal/security/    cgroup、seccomp 与安全策略
internal/util/        进程、临时目录和日志工具
proto/                protobuf 协议及生成代码
deploy/               本地 Kind 的 Deployment 与 Service 清单
examples/             多语言示例和输入输出
```

## API

协议定义见 `proto/sandbox.proto`：

```protobuf
service SandboxService {
  rpc Execute(ExecuteRequest) returns (ExecuteResponse);
}
```

`ExecuteRequest` 可携带语言、源码、标准输入、超时、内存限制和预期输出；响应包含状态、退出码、stdout、stderr、编译错误、耗时和内存用量。未提供语言时默认使用 Go。

默认监听 `0.0.0.0:50051`。标准健康服务名为空字符串。服务启动时先保持 `NOT_SERVING`；只有五语言工具链、临时目录写入和 cgroup 控制器自检全部通过后才切换为 `SERVING`，优雅退出前恢复 `NOT_SERVING`。因此 Kubernetes readiness 和 EndpointSlice 可调度状态与实际执行依赖保持一致。

## 配置

API Server 使用命令行参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-port` | `50051` | gRPC 监听端口 |
| `-temp-dir` | `/tmp/croj-sandbox-local-runs` | 每次执行的临时目录根路径 |
| `-exec-timeout` | `3` | 默认执行超时，单位秒 |
| `-languages` | `go,cpp,python,java,javascript` | 允许的语言列表 |

默认编译超时为 10 秒，默认内存限制为 512 MiB，stdout 与 stderr 各限制 64 KiB。请求可以缩短或调整执行限制；内存请求上限由服务端限制为 4 GiB。

## 本地构建与测试

构建依赖：

- Docker 29+（推荐，避免污染本机工具链）
- 或 Go 1.24、C 编译器、`libseccomp-dev`

```bash
# 编译测试环境和 api-server，不启动服务
docker build --target builder -t coderushoj/croj-sandbox:builder .

# 在构建环境中运行单元测试、竞态检查和静态检查
docker run --rm coderushoj/croj-sandbox:builder go test -race ./...
docker run --rm coderushoj/croj-sandbox:builder go vet ./...

# 构建包含五种语言运行时的最终镜像
docker build -t coderushoj/croj-sandbox:dev .
```

如本机已安装完整依赖，可直接运行：

```bash
go test -race ./...
go vet ./...
go build ./cmd/api-server
```

## Docker 验证

仅在受控的 Linux 开发机上需要人工联调时启动容器。当前 cgroup 实现需要高权限和主机 cgroup 挂载：

```bash
docker run --rm --privileged --pid=host \
  -v /sys/fs/cgroup:/sys/fs/cgroup:rw \
  -p 50051:50051 \
  --entrypoint /usr/bin/nsenter \
  coderushoj/croj-sandbox:dev \
  --cgroup=/proc/1/ns/cgroup -- /app/api-server
```

代码执行会写入 cgroup，并要求 Linux 内核能力。macOS/Windows 上的 Docker VM、rootless Docker 或受限容器中，健康检查可以工作，但实际执行可能因 cgroup 权限不足而降级或失败。不要为了绕过权限问题在生产环境中直接授予无限制的 `--privileged`；应按威胁模型配置专用节点、RuntimeClass 和最小权限。

## Kubernetes 接入

命名空间由 `croj-platform` 统一创建。仓库提供两个副本的 Kind 开发工作负载和 Service；以下命令只用于本地集群：

```bash
# 将本机构建镜像载入 Kind，并显式标记允许运行高权限沙箱的开发节点
kind load docker-image coderushoj/croj-sandbox:dev --name coderushoj
kubectl label node --all coderushoj.dev/sandbox=true --overwrite

# 先验证，再部署
kubectl apply --server-side --dry-run=server -f deploy/
kubectl apply -f deploy/
```

工作负载使用与 Service 一致的标签和命名端口，并采用 Kubernetes 原生 gRPC 探针：

```yaml
metadata:
  labels:
    app.kubernetes.io/name: croj-sandbox
spec:
  containers:
    - name: sandbox
      image: ghcr.io/coderushoj/croj-sandbox:<version>
      ports:
        - name: grpc
          containerPort: 50051
      readinessProbe:
        grpc:
          port: 50051
      livenessProbe:
        grpc:
          port: 50051
```

`deploy/deployment.yaml` 使用 `coderushoj/croj-sandbox:dev`，只面向本机 Kind；正式镜像版本和生产级工作负载仍需在 `croj-platform` Helm chart 中完成安全加固后发布。`croj-judging-server` 只需要 EndpointSlice 的 `list` 权限，不需要访问 Pod 或 Kubernetes Endpoints API。

## 安全边界

当前实现已经有限时、限输出、进程数、内存监控、cgroup 和 seccomp，但仍有明确的加固项：

- 编译器与用户程序目前运行在同一个沙箱 Pod 内
- seccomp 过滤器加载位置和生命周期需要改为只作用于子进程
- 文件系统与网络命名空间尚未形成完整隔离边界
- cgroup 创建需要节点级权限，生产部署必须使用隔离节点池
- 仓库中的 Kind 清单使用 privileged、host PID、主机 cgroup 挂载和 `nsenter`，只能用于本地开发
- 镜像包含多语言工具链，后续应拆分为按语言版本化的运行时镜像

在这些事项完成安全评审前，不应向匿名公网开放 Execute 接口。生产目标是 Kubernetes 专用沙箱节点池、无外网网络策略、只读根文件系统、独立 RuntimeClass，以及可审计的语言镜像供应链。

## 开发约定

- 协议变化必须同步更新生成代码、调度器契约测试和版本日志
- 新语言必须补充编译/运行/超时/错误场景测试与示例
- 新功能从 Issue 分支开发，通过 PR、CI 和代码审查后合并
- 仓库变更记录在 `CHANGELOG.md`，跨仓部署版本统一记录在平台 release notes

项目整体部署与架构文档请参阅 [croj-platform](https://github.com/CodeRushOJ/croj-platform)。
