package util

import (
	"fmt"
	"log"
	"os"
	"strings"
)

// DebugMode 控制是否输出调试日志
var DebugMode = false

// InitDebugMode 初始化调试模式
func InitDebugMode() {
	debugEnv := os.Getenv("CROJ_DEBUG")
	DebugMode = debugEnv != "" && strings.ToLower(debugEnv) != "false" && debugEnv != "0"
}

// DebugLog 只在调试模式下输出日志
func DebugLog(format string, v ...interface{}) {
	if DebugMode {
		log.Printf("[DEBUG] "+format, v...)
	}
}

// InfoLog 输出普通信息日志
func InfoLog(format string, v ...interface{}) {
	log.Printf("[INFO] "+format, v...)
}

// WarnLog 输出警告日志
func WarnLog(format string, v ...interface{}) {
	log.Printf("[WARN] "+format, v...)
}

// ErrorLog 输出错误日志
func ErrorLog(format string, v ...interface{}) {
	log.Printf("[ERROR] "+format, v...)
}

// FatalLog 输出致命错误并退出程序
func FatalLog(format string, v ...interface{}) {
	log.Fatalf("[FATAL] "+format, v...)
}

// PrintDebug 打印调试信息到标准输出
func PrintDebug(format string, v ...interface{}) {
	if DebugMode {
		fmt.Printf("[DEBUG] "+format+"\n", v...)
	}
}
