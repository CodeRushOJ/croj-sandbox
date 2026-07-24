# croj-sandbox

CodeRushOJ 的代码编译与执行节点。服务通过 gRPC 接收评测请求，在 Linux 上为每次执行创建独立临时目录，并结合超时、输出限制、进程监控、cgroup 与 seccomp 返回结构化结果。

> 当前状态：核心执行链路和 Kubernetes 原生服务发现接口已经具备；安全实现仍在持续加固。现阶段适合本地开发、CI 与受控集群验证，不应直接作为面向不可信公网代码的最终安全边界。

## 核心能力

- gRPC `SandboxService.Execute` 执行协议
- 版本化 `SandboxService.ExecuteBatchV1`：一次编译、顺序执行并流式返回最多 256 个测试点
- Go、C++、Python、Java、JavaScript 五种语言运行配置
- 编译超时、执行超时、内存监控与 64 KiB 标准输出/错误限制
- `Accepted`、`Wrong Answer`、`Compile Error`、`Runtime Error`、`Time Limit Exceeded`、`Memory Limit Exceeded`、`Output Limit Exceeded`、`Sandbox Error` 等结果
- 标准 gRPC Health Checking Protocol 和 gRPC reflection
- Pod 内有界执行并发与 `ResourceExhausted` 过载背压
- Kubernetes `Service`/`EndpointSlice` 原生发现，不依赖 ZooKeeper
- Linux cgroup v2、子进程 seccomp、独立非 root UID 与默认禁网的 fail-closed 执行边界

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
  rpc ExecuteBatchV1(ExecuteBatchV1Request) returns (stream ExecuteBatchV1Event);
}
```

`ExecuteRequest` 可携带语言、源码、标准输入、超时、内存限制和预期输出；响应包含状态、退出码、stdout、stderr、编译错误、耗时和内存用量。未提供语言时默认使用 Go。unary protobuf 序列化体积上限为 4 MiB，服务端在 protobuf unmarshal 前按目标消息类型拒绝更大请求；源码、stdin、expected output 还分别有 4 MiB 字节上限。超过这些资源上限返回 gRPC `ResourceExhausted`，且不会调用执行器。

`ExecuteBatchV1` 保留同一份源码和编译产物，在一个 admission slot 内按请求顺序启动独立 case 进程。编译器和每个 case 都通过同一 fail-closed launcher，分别应用执行超时、内存、输出上限、独立非 root UID、Pod 内请求 cgroup、子进程 seccomp 与禁网策略；任何 launcher、cgroup 或 seccomp 初始化失败都会返回 `Sandbox Error`，不会降级为未隔离执行。`stop_on_failure=true` 时首个非 Accepted 结果后停止。exact 使用 `expected_output`，token 使用规范化 token 序列的 `token_expected_sha256`，因此 token 原始隐藏答案不跨入 sandbox，但 sandbox 仍能在首个 token WA 时早停。事件只允许 `CASE_RESULT`、`COMPILE_ERROR` 和最终 `COMPLETED`。

Batch V1 保留 64 MiB protobuf 接收上限，并额外限制聚合 payload（源码、全部 case ID、stdin、expected output 和 token digest）不超过 64 MiB。请求必须包含 1..256 个唯一非空 `case_id`；每个 `case_id` 不超过 256 字节，源码、每个 stdin 和每个 expected output 分别不超过 4 MiB。超出字节数或 case 数返回 `ResourceExhausted`；空/重复 ID、无效 token digest 等格式错误返回 `InvalidArgument`。judging-server 会在 RPC 前做同样的确定性检查。旧 `Execute` 不变，旧客户端可继续使用；judging-server 批量客户端必须和本版本同时发布。

编译产物仅在单个 RPC 的私有临时目录中存在，不跨提交缓存。RPC 正常结束、编译失败、发送失败和 context 取消都清理源码及产物。整个 batch 只占一个 `max-concurrency` slot，满载仍在编译前返回 `ResourceExhausted`。

Batch 总墙钟预算按“编译预算 + case 数 × 单 case 执行预算 + 5 秒”计算，但硬性夹到最多 5 分钟，避免 256 个 case 把单个 RPC 延长到数小时。客户端取消和 deadline 通过 `SandboxAPI.ExecuteContext` 贯穿编译、执行与 batch 生命周期；旧的进程内 `Execute` 方法仅作为 background-context 兼容包装。

编译 stdout/stderr 分别受现有 64 KiB 上限约束，失败响应只在 `compile_error` 携带一次有界诊断；错误分类字段不复制诊断，避免恶意编译输出放大内存和 gRPC 响应。

默认监听 `0.0.0.0:50051`。标准健康服务名为空字符串。服务启动时先保持 `NOT_SERVING`；只有五语言工具链、临时目录写入和 cgroup 控制器自检全部通过后才切换为 `SERVING`，优雅退出前恢复 `NOT_SERVING`。因此 Kubernetes readiness 和 EndpointSlice 可调度状态与实际执行依赖保持一致。

## 配置

API Server 使用命令行参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-port` | `50051` | gRPC 监听端口 |
| `-temp-dir` | `/tmp/croj-sandbox-local-runs` | 每次执行的临时目录根路径 |
| `-exec-timeout` | `3` | 默认执行超时，单位秒 |
| `-languages` | `go,cpp,python,java,javascript` | 允许的语言列表 |
| `-max-concurrency` | 当前进程 `GOMAXPROCS`，至少为 `1` | 每个 Pod 同时进行的编译/执行数，显式值必须大于 `0` |

默认编译超时为 30 秒；Go 因独立 UID、私有构建缓存以及同一 worker 上的并发冷编译使用 90 秒预算。默认内存限制为 512 MiB，stdout 与 stderr 各限制 64 KiB。请求可以缩短或调整执行限制；内存请求上限由服务端限制为 4 GiB。

### 并发与背压

每个 API Server 使用非阻塞 admission semaphore。请求通过语言和 context 校验后尝试占用执行 slot：

- 未达到 `max-concurrency` 时立即进入 `SandboxAPI.Execute`；
- slot 已满时立即返回 gRPC `codes.ResourceExhausted`，不会调用编译或执行链路；
- Batch V1 的 slot 在 stream interceptor 中、generated handler 首次 `RecvMsg` 之前获取；满载时不会读取或反序列化最多 64 MiB 的 batch payload；
- health 和 reflection RPC 不属于执行方法，即使所有执行 slot 已满也不会占用或等待 slot；
- slot 绑定真实执行生命周期，通过 `defer` 在正常返回和 panic 展开时释放；
- unary recovery interceptor 把 handler panic 转换为 `codes.Internal`，避免单个请求终止服务进程；
- 已取消的 context 在进入执行器前返回 `codes.Canceled`。

服务记录结构化键值 `max_concurrency`、`in_flight` 和进程生命周期内累计的 `rejected_total`，不记录源码、stdin 或预期输出。judging-server 应把 `ResourceExhausted` 作为容量信号，选择其他 Ready Endpoint 或做有上限的抖动退避；不得无限即时重试同一 Pod。

默认值取 Go 运行时当前可用并行 CPU。Kubernetes 部署应结合容器 CPU limit 显式设置，例如 2 CPU limit 使用 `-max-concurrency=2`；编译器内存峰值较大时应进一步降低，而不是只扩大队列。

收到终止信号后，服务先把 health 改为 `NOT_SERVING`，再调用 `grpc.GracefulStop` 停止接受新 RPC 并等待已 admission 的执行完成。drain 最多等待 25 秒，随后调用 `grpc.Stop` 强制终止仍未完成的 RPC；这为 Pod 的 30 秒 `terminationGracePeriodSeconds` 预留 5 秒进程/容器清理余量。对应单元测试用可控 context 验证顺序、优雅完成与超时强停，不依赖 sleep 时序。

### 评测数据与日志

源码、stdin、隐藏 expected output、contestant stdout/stderr 和编译器诊断都属于评测 payload，不得进入 sandbox 服务日志。该约束在默认日志和 `CROJ_DEBUG=1` 下完全相同；debug 模式不是数据脱敏的逃生口。

服务端只记录受支持的语言标识、verdict、exit code、有界的耗时/内存/字节计数，以及 `compile_failed`、`output_mismatch` 等稳定分类。编译器诊断仍会通过 `CompileError` 返回给已授权的调用方，stdout/stderr 也仍保留在执行响应中；仓库的 `simple-client` 是交互式消费者，因此会按用户显式调用打印这些响应字段，但 verbose 日志只输出长度与执行元数据。

新增或修改日志时必须遵守以下规则：

- 不展开请求、响应、执行命令、环境变量或临时目录路径；
- 不把底层错误、编译输出或 WA expected/actual 直接传给日志格式化函数；
- 使用稳定 `event`/`category` 和有界数值字段；
- 扩展 sentinel 回归测试，同时覆盖默认和 debug 两条路径。

## 本地构建与测试

构建依赖：

- Docker 29+（推荐，避免污染本机工具链）
- 或 Go 1.24、C 编译器、`libseccomp-dev`

```bash
# 编译测试环境和 api-server，不启动服务
docker build --target builder -t coderushoj/croj-sandbox:builder .

# 在构建环境中运行单元测试、竞态检查和静态检查
docker run --rm coderushoj/croj-sandbox:builder go test -race -timeout=10m ./...
docker run --rm coderushoj/croj-sandbox:builder go vet ./...

# 构建包含五种语言运行时的最终镜像
docker build -t coderushoj/croj-sandbox:dev .
```

如本机已安装完整依赖，可直接运行：

```bash
go test -race -timeout=10m ./...
go vet ./...
go build ./cmd/api-server
go build ./cmd/sandbox-exec
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

代码执行要求统一 cgroup v2，并会在 worker Pod 自身的 cgroup 下创建 `.croj-supervisor` 与 `.croj-jobs/<request>`。macOS/Windows 上的 Docker VM、rootless Docker 或受限容器中，健康检查可以工作，但实际执行会在 cgroup、launcher 或 seccomp 不可用时明确失败，不会降级为未隔离执行。不要为了绕过权限问题在生产环境中直接授予通用业务节点无限制的 `--privileged`；应使用专用污点节点、独立 RuntimeClass 和最小化的 worker 工作负载。

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

平台 Helm follow-up：在 `sandbox.args` 的 nsenter 分隔符和 `/app/api-server` 之后追加 `-max-concurrency=N`，并让 `N` 与 sandbox Pod 的 CPU/内存 limit、语言工具链峰值和期望并行度一致。参数缺失时使用运行时 CPU 默认值；`0` 或负数会使 API Server 启动失败，避免无界或含糊配置。Pod termination grace 不应低于 30 秒：服务内固定使用其中最多 25 秒 drain，剩余时间留给 kubelet 和容器运行时完成退出。

## 安全边界

当前实现已经有限时、限输出、进程数、内存监控、统一 cgroup v2、独立运行 UID、子进程 seccomp、环境清洗和 socket 禁用。launcher 在执行目标命令前等待 supervisor 完成 cgroup 放置；之后降权、设置 `no_new_privs`、加载 seccomp，再通过 `execve` 进入编译器或用户程序。仍有以下纵深防御事项：

- 编译器与用户程序运行在同一个专用沙箱 Pod，但使用不同的请求进程、UID 和 cgroup
- 文件系统与网络命名空间尚未形成完整隔离边界
- cgroup 创建需要节点级权限，生产部署必须使用隔离节点池
- 仓库中的 Kind 清单使用 privileged、host PID、主机 cgroup 挂载和 `nsenter`，只能用于本地开发
- 镜像包含多语言工具链，后续应拆分为按语言版本化的运行时镜像
- 服务日志主动排除源码、测试输入、隐藏答案、程序输出和编译诊断；日志聚合系统不应被视为评测 payload 存储

在这些事项完成安全评审前，不应向匿名公网开放 Execute 接口。生产目标是 Kubernetes 专用沙箱节点池、无外网网络策略、只读根文件系统、独立 RuntimeClass，以及可审计的语言镜像供应链。

## 开发约定

- 协议变化必须同步更新生成代码、调度器契约测试和版本日志
- `ExecuteBatchV1` 的发布顺序是先升级全部 sandbox Pod，再升级 judging-server；回滚时先回滚 judging-server
- 新语言必须补充编译/运行/超时/错误场景测试与示例
- 新功能从 Issue 分支开发，通过 PR、CI 和代码审查后合并
- 仓库变更记录在 `CHANGELOG.md`，跨仓部署版本统一记录在平台 release notes

项目整体部署与架构文档请参阅 [croj-platform](https://github.com/CodeRushOJ/croj-platform)。
