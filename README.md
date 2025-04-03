# croj-sandbox

croj-sandbox 是一个轻量级代码执行沙箱，用于安全地编译和运行 Go 代码。目前在 v0.1 版本，主要在本地环境中运行。

## 功能特点

- 代码编译：在安全的临时环境中编译 Go 代码
- 代码执行：运行编译后的程序并收集结果
- 限制控制：支持编译超时、执行超时、输出大小限制等
- 结果收集：包括标准输出、标准错误、退出码、执行时间等

## 支持的评测结果

- Accepted：代码成功编译并正确执行
- Compile Error：代码编译失败
- Runtime Error：运行时错误（如除零、非零退出码等）
- Time Limit Exceeded：执行超时
- Output Limit Exceeded：输出超过最大限制
- Sandbox Error：沙箱内部错误

## 配置参数

- CompileTimeout: 编译超时时间（默认10秒）
- ExecTimeout: 执行超时时间（默认3秒）
- MaxStdoutSize: 标准输出最大字节数（默认64KB）
- MaxStderrSize: 标准错误最大字节数（默认64KB）
- HostTempDir: 临时文件目录（默认/tmp/croj-sandbox-local-runs）

## 未来计划

- 支持内存限制检测
- Docker容器隔离支持
- 多语言支持