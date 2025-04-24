// cmd/simple-client/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"time"

	pb "github.com/CodeRushOJ/croj-sandbox/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	serverAddr = flag.String("server", "localhost:50051", "gRPC 服务器地址")
	language   = flag.String("lang", "go", "编程语言")
	sourceFile = flag.String("src", "", "源代码文件路径")
	stdinFile  = flag.String("stdin", "", "标准输入文件路径 (可选)")
	timeout    = flag.Int("t", 3, "执行超时时间（秒）")
	memory     = flag.Int("m", 512, "内存限制（MB）")
	verbose    = flag.Bool("v", false, "详细模式，显示更多调试信息")
)

func main() {
	flag.Parse()

	// 设置日志格式
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	if *sourceFile == "" {
		log.Fatal("错误：必须提供源代码文件路径 (-src)")
	}

	// 读取源代码文件
	sourceCodeBytes, err := ioutil.ReadFile(*sourceFile)
	if err != nil {
		log.Fatalf("读取源代码文件 '%s' 失败: %v", *sourceFile, err)
	}
	sourceCode := string(sourceCodeBytes)

	// 读取标准输入文件（如果提供）
	var stdinContent string
	if *stdinFile != "" {
		stdinBytes, err := ioutil.ReadFile(*stdinFile)
		if err != nil {
			log.Fatalf("读取标准输入文件 '%s' 失败: %v", *stdinFile, err)
		}
		stdinContent = string(stdinBytes)
	}

	// 设置 gRPC 连接选项
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()), // 使用不安全的连接（仅用于测试）
		grpc.WithBlock(), // 阻塞直到连接建立
	}

	// 连接到 gRPC 服务器
	log.Printf("连接到 gRPC 服务器 %s...", *serverAddr)
	conn, err := grpc.Dial(*serverAddr, opts...)
	if err != nil {
		log.Fatalf("连接服务器失败: %v", err)
	}
	defer conn.Close()

	log.Println("已连接到服务器")

	// 创建 gRPC 客户端
	client := pb.NewSandboxServiceClient(conn)

	// 准备 gRPC 请求
	req := &pb.ExecuteRequest{
		Language:    *language,
		SourceCode:  sourceCode,
		Stdin:       stdinContent,
		Timeout:     int32(*timeout),
		MemoryLimit: int32(*memory),
	}

	// 设置请求上下文和超时
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout+5)*time.Second) // 客户端超时比服务器稍长
	defer cancel()

	// 调用 gRPC 服务
	log.Printf("发送执行请求: Language=%s, Timeout=%ds, Memory=%dMB", req.Language, req.Timeout, req.MemoryLimit)
	startTime := time.Now()
	resp, err := client.Execute(ctx, req)
	elapsedTime := time.Since(startTime)

	if err != nil {
		log.Fatalf("调用 Execute 方法失败: %v", err)
	}

	// 打印结果
	fmt.Println("--- 执行结果 ---")
	fmt.Printf("状态: %s\n", resp.Status)
	fmt.Printf("退出码: %d\n", resp.ExitCode)
	fmt.Printf("执行时间: %d ms\n", resp.TimeUsed)
	fmt.Printf("内存使用: %d KB\n", resp.MemoryUsed)
	fmt.Printf("客户端请求耗时: %s\n", elapsedTime)

	if resp.Stdout != "" {
		fmt.Println("--- 标准输出 ---")
		fmt.Println(resp.Stdout)
	}
	if resp.Stderr != "" {
		fmt.Println("--- 标准错误 ---")
		fmt.Println(resp.Stderr)
	}
	if resp.CompileError != "" {
		fmt.Println("--- 编译错误 ---")
		fmt.Println(resp.CompileError)
	}
	if resp.Error != "" {
		fmt.Println("--- 内部错误 ---")
		fmt.Println(resp.Error)
	}

	if *verbose {
		log.Println("--- 详细响应信息 ---")
		log.Printf("%+v\n", resp)
	}

	log.Println("客户端执行完毕")
}
