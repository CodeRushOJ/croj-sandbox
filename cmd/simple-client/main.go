// cmd/simple-client/main.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/CodeRushOJ/croj-sandbox/internal/sandbox"
)

var (
	sourceFile = flag.String("source", "", "Go源代码文件路径")
	stdinFile  = flag.String("stdin", "", "输入数据文件路径")
	timeout    = flag.Int("timeout", 3, "执行超时时间（秒）")
)

func main() {
	flag.Parse()
	
	// 设置日志格式
	log.SetFlags(log.Ldate | log.Ltime)
	
	// 验证参数
	if *sourceFile == "" {
		flag.Usage()
		log.Fatal("必须指定源代码文件路径")
	}
	
	// 读取源代码
	sourceCode, err := os.ReadFile(*sourceFile)
	if err != nil {
		log.Fatalf("无法读取源代码文件: %v", err)
	}
	
	// 读取标准输入（如果提供）
	var stdin *string
	if *stdinFile != "" {
		stdinData, err := os.ReadFile(*stdinFile)
		if err != nil {
			log.Fatalf("无法读取标准输入文件: %v", err)
		}
		stdinStr := string(stdinData)
		stdin = &stdinStr
	}
	
	// 创建API实例
	api, err := sandbox.NewSandboxAPI()
	if err != nil {
		log.Fatalf("初始化API失败: %v", err)
	}
	defer api.Close()
	
	// 创建执行请求
	request := sandbox.Request{
		SourceCode: string(sourceCode),
		Stdin:      stdin,
		Timeout:    timeout,
	}
	
	// 执行代码
	response := api.Execute(request)
	
	// 打印结果
	fmt.Printf("执行状态: %s\n", response.Status)
	fmt.Printf("退出码: %d\n", response.ExitCode)
	fmt.Printf("执行时间: %d ms\n", response.TimeUsed)
	
	if response.CompileError != "" {
		fmt.Printf("编译错误:\n%s\n", response.CompileError)
	}
	
	if response.Stdout != "" {
		fmt.Printf("标准输出:\n%s\n", response.Stdout)
	}
	
	if response.Stderr != "" {
		fmt.Printf("标准错误:\n%s\n", response.Stderr)
	}
	
	if response.Error != "" {
		fmt.Printf("错误信息: %s\n", response.Error)
	}
	
	// 打印结果为JSON格式
	prettyJSON, _ := json.MarshalIndent(response, "", "  ")
	fmt.Printf("\n完整结果JSON:\n%s\n", string(prettyJSON))
}
