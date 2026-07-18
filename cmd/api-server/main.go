// cmd/api-server/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/CodeRushOJ/croj-sandbox/internal/sandbox"
	pb "github.com/CodeRushOJ/croj-sandbox/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

var (
	port      = flag.Int("port", 50051, "gRPC 服务端口") // 默认 gRPC 端口
	tempDir   = flag.String("temp-dir", "", "临时目录路径，为空则使用默认路径")
	execTime  = flag.Int("exec-timeout", 3, "执行超时时间（秒）")
	languages = flag.String("languages", "go,cpp,python,java,javascript", "支持的语言列表（逗号分隔）")
)

// server 结构体实现了 SandboxServiceServer 接口
type server struct {
	pb.UnimplementedSandboxServiceServer
	api            *sandbox.SandboxAPI
	supportedLangs []string
}

func newGRPCServer(api *sandbox.SandboxAPI, supportedLangs []string) (*grpc.Server, *health.Server) {
	grpcServer := grpc.NewServer()
	pb.RegisterSandboxServiceServer(grpcServer, &server{
		api:            api,
		supportedLangs: supportedLangs,
	})
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	reflection.Register(grpcServer)
	return grpcServer, healthServer
}

// Execute 方法实现了 gRPC 服务接口
func (s *server) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	log.Printf("收到 gRPC 请求: Language=%s", req.Language)

	// 验证语言是否支持
	langSupported := false
	requestLang := req.Language
	if requestLang == "" {
		requestLang = "go" // 默认语言
	}
	for _, lang := range s.supportedLangs {
		if requestLang == lang {
			langSupported = true
			break
		}
	}

	if !langSupported {
		log.Printf("不支持的语言: %s", requestLang)
		return &pb.ExecuteResponse{
			Status: "Error",
			Error:  fmt.Sprintf("不支持的编程语言: %s", requestLang),
		}, nil // 返回错误信息，但不返回 gRPC 错误
	}

	// 构造 sandbox 请求
	sandboxReq := sandbox.Request{
		Language:   requestLang,
		SourceCode: req.SourceCode,
		// Stdin, Timeout, MemoryLimit, ExpectedOutput are pointers, handle nil
	}
	if req.Stdin != "" {
		stdinVal := req.Stdin // Create a local variable to take the address
		sandboxReq.Stdin = &stdinVal
	}
	if req.Timeout > 0 {
		timeoutVal := int(req.Timeout)
		sandboxReq.Timeout = &timeoutVal
	}
	if req.MemoryLimit > 0 {
		memLimitVal := int(req.MemoryLimit)
		sandboxReq.MemoryLimit = &memLimitVal
	}
	if req.ExpectedOutput != "" {
		expectedOutputVal := req.ExpectedOutput // Create a local variable
		sandboxReq.ExpectedOutput = &expectedOutputVal
	}

	// 注意：默认值处理已移至 sandbox.API.Execute 内部，这里不再需要设置默认值
	// Timeout 和 MemoryLimit 在 sandbox.API.Execute 中会根据 cfg 和请求中的值决定

	// 执行代码
	resp := s.api.Execute(sandboxReq)

	// 转换 sandbox 响应为 gRPC 响应
	grpcResp := &pb.ExecuteResponse{
		Status:       resp.Status,
		ExitCode:     int32(resp.ExitCode),
		Stdout:       resp.Stdout,
		Stderr:       resp.Stderr,
		Error:        resp.Error,
		CompileError: resp.CompileError,
		TimeUsed:     resp.TimeUsed,   // Already in milliseconds
		MemoryUsed:   resp.MemoryUsed, // Already in KB
	}

	log.Printf("执行完成: Status=%s, Time=%dms, Memory=%dKB", grpcResp.Status, grpcResp.TimeUsed, grpcResp.MemoryUsed)
	return grpcResp, nil
}

func main() {
	flag.Parse()

	// 设置日志格式
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("启动 croj-sandbox gRPC 服务 (端口: %d)", *port)

	// 解析支持的语言列表
	supportedLangs := strings.Split(*languages, ",")
	for i, lang := range supportedLangs {
		supportedLangs[i] = strings.TrimSpace(lang)
	}
	log.Printf("支持的编程语言: %v", supportedLangs)

	// 创建自定义配置
	cfg := sandbox.DefaultConfig()
	if *tempDir != "" {
		cfg.HostTempDir = *tempDir
	}
	cfg.DefaultExecuteTimeLimit = time.Duration(*execTime) * time.Second
	cfg.DefaultExecuteMemoryLimit = 512 * 1024 * 1024 // 设置一个默认内存限制 (512MB in bytes)，如果需要可以从flag读取

	// 初始化API
	api, err := sandbox.NewSandboxAPIWithConfig(cfg)
	if err != nil {
		log.Fatalf("初始化 Sandbox API 失败: %v", err)
	}
	defer api.Close()

	// 启动 gRPC 服务器
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatalf("监听端口 %d 失败: %v", *port, err)
	}

	grpcServer, healthServer := newGRPCServer(api, supportedLangs)
	if err := markServingAfterCheck(healthServer, func() error {
		return checkStartupDependencies(cfg, supportedLangs)
	}); err != nil {
		log.Fatalf("沙箱启动自检失败: %v", err)
	}

	log.Printf("gRPC 服务器正在监听端口 %d", *port)

	// 优雅关闭
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("接收到关闭信号，停止 gRPC 服务...")
		healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
		grpcServer.GracefulStop()
		log.Println("gRPC 服务已成功关闭")
	}()

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("启动 gRPC 服务失败: %v", err)
	}
}
