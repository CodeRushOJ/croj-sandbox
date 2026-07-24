# Changelog

本文件记录 croj-sandbox 的重要功能、兼容性与部署变化。版本遵循 Semantic Versioning；尚未发布的内容保留在 `Unreleased`。

## Unreleased

### Added

- 增加版本化 server-streaming `ExecuteBatchV1`：一个请求只编译一次，按序返回最多 256 个测试点结果并保留旧 `Execute` 兼容接口
- 增加 batch 单次 admission、context 取消、编译产物确定性清理和 stream panic recovery
- 增加 compile-counter、事件顺序、清理与流式 handler 回归测试
- 增加按 Pod 限制的 `max-concurrency` admission semaphore；默认取可用并行 CPU，显式值必须大于 0
- 满载请求返回 gRPC `ResourceExhausted` 且不进入执行器，并记录 `max_concurrency`、`in_flight`、`rejected_total`
- 增加 handler panic recovery，以及并发上限、拒绝、slot 释放、取消和竞态测试
- 增加 message-type aware protobuf codec：unary `ExecuteRequest` 在 unmarshal 前限制为 4 MiB，Batch V1 保留 64 MiB
- 增加源码、stdin、expected output、case ID、case 数和 batch 聚合 payload 的明确字节/数量上限
- 增加 `SandboxAPI.ExecuteContext`，gRPC 取消和 deadline 贯穿编译/执行，旧 `Execute` 保留兼容包装
- 增加 5 分钟 Batch V1 总墙钟硬上限
- 增加 25 秒有界关闭：先发布 `NOT_SERVING`，超时后从 `GracefulStop` 切换到强制 `Stop`
- Kubernetes 原生 `Service`/EndpointSlice 发现契约和 `grpc:50051` 命名端口
- 标准 gRPC health/reflection，以及工具链、临时目录和真实 cgroup 迁移启动自检
- 双副本 Kind 开发清单、原生 gRPC probes、CI、竞态测试和 cgroup 集成测试
- 五语言最终镜像工具链校验：Go、C++、Python、Java、JavaScript
- 增加短生命周期 `sandbox-exec` launcher，在目标进程进入请求 cgroup 后降为独立 UID、设置 `no_new_privs`、加载 seccomp 并清洗环境
- 增加特权 Linux 集成测试，验证非 root、零有效 capability、seccomp、请求 cgroup、环境隔离和 socket 禁用

### Changed

- Kind 开发 Deployment 按 2 CPU limit 显式设置 `-max-concurrency=2`
- Batch V1 admission 移到 stream interceptor，在 generated handler `RecvMsg` 前拒绝满载请求；health/reflection 不占执行 slot
- 删除 ZooKeeper 注册与客户端发现，调度统一交给 Kubernetes EndpointSlice
- cgroup v2 在父级正确委派 memory/cpu/pids controller，并按 Pod UID 隔离 cgroup 名称
- Go 构建与运行工具链升级至 1.26.5，Java 镜像从 JRE 改为 JDK 17
- gRPC 升级至 1.79.3、`x/net` 升级至 0.55.0，修复 GitHub Dependabot 报告的 1 个 Critical 与 3 个 Medium 漏洞
- sandbox 日志改为稳定的 `event`/`category` 与有界指标；verbose CLI 日志只显示响应字段长度和执行元数据
- 编译器与用户程序统一通过 fail-closed 执行器；cgroup、launcher 或 seccomp 不可用时不再降级执行
- 请求 cgroup 固定创建在 worker Pod 自身 cgroup 下，并要求统一 cgroup v2

### Fixed

- 将默认编译预算提高到 30 秒，并为 Go 并发冷编译提供 90 秒预算，避免独立 UID 与私有构建缓存下的合法编译被误判为超时
- 将编译器的 1 GiB 内存预算与参赛程序运行内存拆分，避免低内存题目或新版工具链在编译阶段被 cgroup OOM 杀死
- 修复进程监控在后台 goroutine 仍写入统计数据时提前返回导致的竞态，并确保自然退出、超时和取消路径均返回最终快照
- 修复 seccomp 曾在 API Server 线程而非目标子进程中加载、初始化失败仍继续执行的问题
- 修复编译器绕过 cgroup、seccomp、网络、输出和身份边界直接运行的问题
- 修复跨请求共享 cgroup cleanup 状态导致的竞态与误清理风险
- 限制 batch 编译诊断内存并去除 `error`/`compile_error` 重复载荷，避免大输出触发 OOM 或错误重试
- token checker 改用规范化 token 序列 SHA-256 在 sandbox 内判定，原始 expected 不跨服务边界且恢复首错早停
- 修复 cgroup v2 清理被误判为 v1 的问题
- 修复不同 Pod 内相同 namespace PID 可能复用主机 cgroup 的问题
- 修复最终镜像缺少 Go 编译器和 `javac` 的问题
- 修复编译失败日志泄露编译诊断或源码片段，以及 Wrong Answer 日志泄露隐藏 expected output 和 contestant stdout 的问题
- 增加默认与 `CROJ_DEBUG` 模式的 sentinel 回归测试，覆盖 runner、legacy compiler、executor、gRPC 和临时目录日志边界
- 修复命令启动失败时 stdin 写入 goroutine 可能晚于执行器返回的问题，确保失败路径没有悬空日志写入

## 0.2.0 - 2025-04-25

- 引入 gRPC `SandboxService.Execute` 协议和多节点服务注册原型
- 增加简单 gRPC 客户端、示例脚本和 Docker 镜像入口
- 在早期架构中使用 ZooKeeper 进行节点注册与选择

## 0.1.0 - 2025-04-04

- 建立本地代码编译、运行、输入输出比对和结果模型
- 支持 Go、C++、Python、Java 和 JavaScript
- 增加执行超时、输出限制、内存监控、cgroup 与 seccomp 基础能力
- 提供多语言示例与资源限制测试程序
