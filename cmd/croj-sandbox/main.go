// cmd/croj-sandbox/main.go
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/CodeRushOJ/croj-sandbox/internal/sandbox"
	"github.com/CodeRushOJ/croj-sandbox/internal/util"
)

var (
	language = flag.String("lang", "go", "编程语言 (go, cpp, python, java, javascript)")
	timeLimit = flag.Int("time", 3, "执行时间限制（秒）")
	memLimit = flag.Int("mem", 512, "内存限制（MB）")
)

func main() {
	flag.Parse()
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ltime | log.Lshortfile)

	// --- 配置 ---
	cfg := sandbox.DefaultConfig()
	cfg.DefaultExecuteTimeLimit = time.Duration(*timeLimit) * time.Second
	
	// 确保选择的语言受支持
	if _, ok := cfg.Languages[*language]; !ok {
		log.Fatalf("不支持的语言: %s", *language)
	}

	// --- 初始化沙盒 ---
	runner, err := sandbox.NewRunner(cfg)
	if err != nil {
		log.Fatalf("初始化沙盒失败: %v", err)
	}
	defer runner.Close()

	// --- 测试用例 ---
	testCases := getTestCases()
	
	// 获取特定语言的测试用例
	langTests, ok := testCases[*language]
	if !ok {
		log.Fatalf("没有找到语言 %s 的测试用例", *language)
	}

	// --- 运行测试用例 ---
	for name, tc := range langTests {
		fmt.Printf("\n--- 运行测试用例: [%s - %s] ---\n", *language, name)
		ctx, cancel := context.WithTimeout(context.Background(), 
			cfg.DefaultCompileTimeLimit + cfg.DefaultExecuteTimeLimit + 5*time.Second)

		// 显示是否有预期输出
		if tc.expectedOutput != nil {
			fmt.Printf("提供了预期输出，将比较结果\n")
		}

		result := runner.Run(ctx, *language, tc.code, tc.stdin, tc.expectedOutput)

		// --- 打印结果 ---
		fmt.Printf("状态: %s\n", result.Status)
		fmt.Printf("退出码: %d\n", result.ExitCode)
		fmt.Printf("用时: %d 毫秒\n", result.TimeUsedMillis)
		fmt.Printf("内存: %d KB (未测量)\n", result.MemoryUsedKB)

		if result.Status == sandbox.StatusCompileError {
			fmt.Printf("编译输出:\n%s\n", result.CompileOutput)
			if result.Error != "" && result.Error != result.CompileOutput { 
				fmt.Printf("编译错误详情: %s\n", result.Error)
			}
		} else {
			// 处理输出比较的特殊情况
			if result.Status == sandbox.StatusWrongAnswer && tc.expectedOutput != nil {
				fmt.Printf("\n=== 输出比较 ===\n")
				fmt.Printf("预期输出:\n%s\n", *tc.expectedOutput)
				fmt.Printf("实际输出:\n%s\n", result.Stdout)
				fmt.Printf("--- 规范化后比较 ---\n")
				fmt.Printf("预期: %q\n", util.NormalizeString(*tc.expectedOutput))
				fmt.Printf("实际: %q\n", util.NormalizeString(result.Stdout))
			} else {
				if result.Stderr != "" {
					fmt.Printf("标准错误输出 (%d 字节):\n%s\n", len(result.Stderr), result.Stderr)
				} else {
					fmt.Println("标准错误输出: (空)")
				}
				
				if result.Stdout != "" {
					maxDisplay := 256
					stdoutDisplay := result.Stdout
					if len(stdoutDisplay) > maxDisplay {
						stdoutDisplay = stdoutDisplay[:maxDisplay] + "..."
					}
					fmt.Printf("标准输出 (%d 字节):\n%s\n", len(result.Stdout), stdoutDisplay)
				} else {
					fmt.Println("标准输出: (空)")
				}

				if result.Error != "" && result.Status != sandbox.StatusAccepted {
					fmt.Printf("错误详情: %s\n", result.Error)
				}
			}
		}
		fmt.Println("------------------------------------")
		cancel() // 取消本次运行的上下文
	}
}

// TestCase 定义一个测试用例
type TestCase struct {
	code           string
	stdin          *string
	expectedOutput *string
}

// getTestCases 返回所有支持语言的测试用例
func getTestCases() map[string]map[string]TestCase {
	allTests := make(map[string]map[string]TestCase)
	
	// Go 语言测试用例
	allTests["go"] = map[string]TestCase{
		"简单输出": {
			code: `
package main
import "fmt"
func main() { fmt.Println("Hello Go!") }`,
			stdin: nil,
			expectedOutput: stringPtr("Hello Go!"),
		},
		"读取输入": {
			code: `
package main
import ("fmt"; "io"; "os")
func main() {
	input, _ := io.ReadAll(os.Stdin)
	fmt.Printf("收到: %s", string(input))
}`,
			stdin: stringPtr("Go 测试输入\n"),
			expectedOutput: nil,
		},
	}
	
	// C++ 测试用例
	allTests["cpp"] = map[string]TestCase{
		"简单输出": {
			code: `
#include <iostream>
int main() {
	std::cout << "Hello C++!" << std::endl;
	return 0;
}`,
			stdin: nil,
			expectedOutput: stringPtr("Hello C++!"),
		},
		"读取输入": {
			code: `
#include <iostream>
#include <string>
int main() {
	std::string input;
	std::getline(std::cin, input);
	std::cout << "收到: " << input << std::endl;
	return 0;
}`,
			stdin: stringPtr("C++ 测试输入"),
			expectedOutput: nil,
		},
	}
	
	// Python 测试用例
	allTests["python"] = map[string]TestCase{
		"简单输出": {
			code: `
print("Hello Python!")`,
			stdin: nil,
			expectedOutput: stringPtr("Hello Python!"),
		},
		"读取输入": {
			code: `
input_data = input()
print(f"收到: {input_data}")`,
			stdin: stringPtr("Python 测试输入"),
			expectedOutput: nil,
		},
	}
	
	// Java 测试用例
	allTests["java"] = map[string]TestCase{
		"简单输出": {
			code: `
public class Main {
	public static void main(String[] args) {
		System.out.println("Hello Java!");
	}
}`,
			stdin: nil,
			expectedOutput: stringPtr("Hello Java!"),
		},
		"读取输入": {
			code: `
import java.util.Scanner;

public class Main {
	public static void main(String[] args) {
		Scanner scanner = new Scanner(System.in);
		String input = scanner.nextLine();
		System.out.println("收到: " + input);
		scanner.close();
	}
}`,
			stdin: stringPtr("Java 测试输入"),
			expectedOutput: nil,
		},
	}
	
	// JavaScript 测试用例
	allTests["javascript"] = map[string]TestCase{
		"简单输出": {
			code: `
console.log("Hello JavaScript!");`,
			stdin: nil,
			expectedOutput: stringPtr("Hello JavaScript!"),
		},
	}
	
	return allTests
}

// 辅助函数：获取字符串指针
func stringPtr(s string) *string {
	return &s
}