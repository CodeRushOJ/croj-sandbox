package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/CodeRushOJ/croj-sandbox/internal/sandbox"
	"github.com/CodeRushOJ/croj-sandbox/internal/util"
)

var (
	sourceFile = flag.String("source", "", "源代码文件路径")
	language   = flag.String("lang", "", "编程语言 (go, cpp, python等，如果未指定，将从文件扩展名推断)")
	stdinFile  = flag.String("stdin", "", "输入数据文件路径")
	outputFile = flag.String("output", "", "预期输出文件路径")
	timeout    = flag.Int("timeout", 3, "执行超时时间（秒）")
	memLimit   = flag.Int("mem", 512, "内存限制（MB）")
	apiURL     = flag.String("api", "", "远程API URL (如果提供，则使用远程执行，否则使用本地执行)")
	verbose    = flag.Bool("v", false, "详细模式，显示更多调试信息")
)

// 从文件扩展名推断语言
func inferLanguage(filename string) string {
	ext := strings.ToLower(strings.TrimPrefix(filename[strings.LastIndex(filename, "."):], "."))
	switch ext {
	case "go":
		return "go"
	case "c", "cpp", "cc", "cxx":
		return "cpp"
	case "py":
		return "python"
	case "java":
		return "java"
	case "js":
		return "javascript"
	default:
		return ""
	}
}

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
	
	// 确定编程语言
	lang := *language
	if lang == "" {
		lang = inferLanguage(*sourceFile)
		if lang == "" {
			log.Fatal("无法从文件扩展名推断语言，请使用 -lang 参数指定")
		}
		fmt.Printf("从文件扩展名推断语言: %s\n", lang)
	}
	
	// 读取标准输入（如果提供）
	var stdin *string
	if *stdinFile != "" {
		fmt.Printf("使用输入文件: %s\n", *stdinFile)
		stdinData, err := os.ReadFile(*stdinFile)
		if err != nil {
			log.Fatalf("无法读取标准输入文件: %v", err)
		}
		stdinStr := string(stdinData)
		stdin = &stdinStr
	}
	
	// 读取期望输出（如果提供）
	var expectedOutput *string
	if *outputFile != "" {
		fmt.Printf("使用预期输出文件: %s (将比较执行结果)\n", *outputFile)
		outputData, err := os.ReadFile(*outputFile)
		if err != nil {
			log.Fatalf("无法读取预期输出文件: %v", err)
		}
		outputStr := string(outputData)
		expectedOutput = &outputStr
	}
	
	// 创建执行请求
	request := sandbox.Request{
		Language:       lang,
		SourceCode:     string(sourceCode),
		Stdin:          stdin,
		Timeout:        timeout,
		MemoryLimit:    memLimit,
		ExpectedOutput: expectedOutput,
	}
	
	var response sandbox.Response
	
	// 检查是使用本地执行还是远程API执行
	if *apiURL != "" {
		// 远程API执行
		response = executeRemote(request, *apiURL)
	} else {
		// 本地执行
		response = executeLocal(request)
	}
	
	// 打印结果
	fmt.Printf("\n=== 执行结果 ===\n")
	fmt.Printf("状态: %s\n", response.Status)
	fmt.Printf("退出码: %d\n", response.ExitCode)
	fmt.Printf("执行时间: %d ms\n", response.TimeUsed)
	
	// 显示内存使用信息
	if response.MemoryUsed > 0 {
		fmt.Printf("内存使用: %d KB (限制: %d MB)\n", response.MemoryUsed, *memLimit)
	} else {
		fmt.Printf("内存使用: 未测量 (限制: %d MB)\n", *memLimit)
	}
	
	// 检查是否是Wrong Answer
	if response.Status == string(sandbox.StatusWrongAnswer) {
		fmt.Printf("\n=== 输出比较 ===\n")
		fmt.Printf("预期输出:\n%s\n", *expectedOutput)
		fmt.Printf("实际输出:\n%s\n", response.Stdout)
		fmt.Printf("\n输出不匹配! 请检查以上内容的差异。\n")
		
		// 如果在详细模式下，显示规范化后的字符串比较
		if *verbose {
			normalizedExpected := util.NormalizeString(*expectedOutput)
			normalizedActual := util.NormalizeString(response.Stdout)
			fmt.Printf("\n规范化后的预期输出:\n%s\n", normalizedExpected)
			fmt.Printf("规范化后的实际输出:\n%s\n", normalizedActual)
		}
	}
	
	if response.CompileError != "" {
		fmt.Printf("\n=== 编译错误 ===\n%s\n", response.CompileError)
	}
	
	if response.Stdout != "" && response.Status != string(sandbox.StatusWrongAnswer) {
		fmt.Printf("\n=== 标准输出 ===\n%s\n", response.Stdout)
	}
	
	if response.Stderr != "" {
		fmt.Printf("\n=== 标准错误 ===\n%s\n", response.Stderr)
	}
	
	if response.Error != "" && response.Error != response.CompileError {
		fmt.Printf("\n=== 错误信息 ===\n%s\n", response.Error)
	}
	
	// 在详细模式下，打印JSON格式的完整结果
	if *verbose {
		prettyJSON, _ := json.MarshalIndent(response, "", "  ")
		fmt.Printf("\n=== 完整结果JSON ===\n%s\n", string(prettyJSON))
	}
}

// 使用本地沙箱执行代码
func executeLocal(req sandbox.Request) sandbox.Response {
	// 创建API实例
	api, err := sandbox.NewSandboxAPI()
	if err != nil {
		log.Fatalf("初始化本地沙箱失败: %v", err)
	}
	defer api.Close()
	
	fmt.Printf("使用本地沙箱执行 %s 代码...\n", req.Language)
	return api.Execute(req)
}

// 向远程API发送请求执行代码
func executeRemote(req sandbox.Request, apiURL string) sandbox.Response {
	reqJSON, err := json.Marshal(req)
	if err != nil {
		log.Fatalf("序列化请求失败: %v", err)
	}
	
	fmt.Printf("向远程API发送 %s 代码执行请求: %s\n", req.Language, apiURL)
	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(reqJSON))
	if err != nil {
		log.Fatalf("请求API失败: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("API返回错误状态码 %d: %s", resp.StatusCode, string(body))
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("读取API响应失败: %v", err)
	}
	
	var response sandbox.Response
	if err := json.Unmarshal(body, &response); err != nil {
		log.Fatalf("解析API响应失败: %v", err)
	}
	
	return response
}
