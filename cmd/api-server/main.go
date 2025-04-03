// cmd/api-server/main.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CodeRushOJ/croj-sandbox/internal/sandbox"
)

var (
	port     = flag.Int("port", 8080, "API服务端口")
	tempDir  = flag.String("temp-dir", "", "临时目录路径，为空则使用默认路径")
	execTime = flag.Int("exec-timeout", 3, "执行超时时间（秒）")
)

func main() {
	flag.Parse()
	
	// 设置日志格式
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	log.Printf("启动 croj-sandbox API 服务 (端口: %d)", *port)
	
	// 创建自定义配置
	cfg := sandbox.DefaultConfig()
	if *tempDir != "" {
		cfg.HostTempDir = *tempDir
	}
	cfg.ExecTimeout = time.Duration(*execTime) * time.Second
	
	// 初始化API
	api, err := sandbox.NewSandboxAPIWithConfig(cfg)
	if err != nil {
		log.Fatalf("初始化API失败: %v", err)
	}
	defer api.Close()
	
	// 创建HTTP处理器
	http.HandleFunc("/execute", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "仅支持POST请求", http.StatusMethodNotAllowed)
			return
		}
		
		// 读取请求体
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "读取请求失败", http.StatusBadRequest)
			return
		}
		
		// 解析请求参数
		var req sandbox.Request
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "无效的JSON格式", http.StatusBadRequest)
			return
		}
		
		// 执行代码
		response := api.Execute(req)
		
		// 返回结果
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})
	
	// 添加健康检查端点
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "API服务正常运行中")
	})
	
	// 启动服务器
	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	
	// 优雅关闭
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("接收到关闭信号，停止服务...")
		server.Close()
	}()
	
	// 启动HTTP服务
	log.Printf("API服务器运行在 http://localhost:%d", *port)
	log.Printf("可以使用以下命令测试:  curl -X POST http://localhost:%d/execute -H \"Content-Type: application/json\" -d '{\"sourceCode\":\"package main\\nimport \\\"fmt\\\"\\nfunc main() {\\n  fmt.Println(\\\"Hello API\\\")\\n}\"}'", *port)
	
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("HTTP服务器错误: %v", err)
	}
	
	log.Println("API服务器已成功关闭")
}
