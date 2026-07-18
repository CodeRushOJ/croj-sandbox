# Changelog

本文件记录 croj-sandbox 的重要功能、兼容性与部署变化。版本遵循 Semantic Versioning；尚未发布的内容保留在 `Unreleased`。

## Unreleased

### Added

- 增加按 Pod 限制的 `max-concurrency` admission semaphore；默认取可用并行 CPU，显式值必须大于 0
- 满载请求返回 gRPC `ResourceExhausted` 且不进入执行器，并记录 `max_concurrency`、`in_flight`、`rejected_total`
- 增加 handler panic recovery，以及并发上限、拒绝、slot 释放、取消和竞态测试
- Kubernetes 原生 `Service`/EndpointSlice 发现契约和 `grpc:50051` 命名端口
- 标准 gRPC health/reflection，以及工具链、临时目录和真实 cgroup 迁移启动自检
- 双副本 Kind 开发清单、原生 gRPC probes、CI、竞态测试和 cgroup 集成测试
- 五语言最终镜像工具链校验：Go、C++、Python、Java、JavaScript

### Changed

- Kind 开发 Deployment 按 2 CPU limit 显式设置 `-max-concurrency=2`
- 删除 ZooKeeper 注册与客户端发现，调度统一交给 Kubernetes EndpointSlice
- cgroup v2 在父级正确委派 memory/cpu/pids controller，并按 Pod UID 隔离 cgroup 名称
- Go 构建与运行工具链升级至 1.24.6，Java 镜像从 JRE 改为 JDK 17
- sandbox 日志改为稳定的 `event`/`category` 与有界指标；verbose CLI 日志只显示响应字段长度和执行元数据

### Fixed

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
