// internal/util/tempdir.go
package util

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"os/exec"

	"github.com/google/uuid"
)

// SetupHostRunDir creates a unique temporary directory for a run on the host.
// It returns the path to the created directory and a cleanup function.
func SetupHostRunDir(baseDir string) (runDir string, cleanup func(), err error) {
	runID := uuid.New().String()
	runDir = filepath.Join(baseDir, runID)

	// Ensure base directory exists
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", nil, fmt.Errorf("failed to create base temp directory %s: %w", baseDir, err)
	}

	// Create the specific run directory
	if err := os.Mkdir(runDir, 0755); err != nil {
		return "", nil, fmt.Errorf("failed to create host run temp dir %s: %w", runDir, err)
	}
	log.Printf("Created host temp dir: %s", runDir)

	cleanup = func() {
		if err := os.RemoveAll(runDir); err != nil {
			log.Printf("Warning: failed to clean up host temp dir %s: %v", runDir, err)
		} else {
			log.Printf("Cleaned up host temp dir: %s", runDir)
		}
	}

	return runDir, cleanup, nil
}

// EnsureDir creates a directory if it doesn't exist
func EnsureDir(dirName string) error {
    err := os.MkdirAll(dirName, 0755)
    if err != nil {
        return fmt.Errorf("failed to create directory %s: %w", dirName, err)
    }
    return nil
}

// ProcessCommandString replaces placeholders in a command string with actual values
func ProcessCommandString(cmdTemplate string, replacements map[string]string) string {
	result := cmdTemplate
	for placeholder, value := range replacements {
		result = strings.ReplaceAll(result, placeholder, value)
	}
	return result
}

// ProcessCommandTemplate processes a command template and returns command parts for exec.Command
func ProcessCommandTemplate(cmdTemplate string, replacements map[string]string) ([]string, error) {
	cmdStr := ProcessCommandString(cmdTemplate, replacements)
	if cmdStr == "" {
		return nil, fmt.Errorf("empty command after processing")
	}
	
	// 简单拆分命令，将命令拆为数组 (不处理复杂引号)
	cmdParts := strings.Fields(cmdStr)
	if len(cmdParts) == 0 {
		return nil, fmt.Errorf("no command parts after splitting")
	}
	
	return cmdParts, nil
}

// CompareOutputs compares actual output with expected output
// Normalizes both strings by trimming whitespace and normalizing line endings
func CompareOutputs(actual, expected string) bool {
	// 标准化字符串
	actual = NormalizeString(actual)
	expected = NormalizeString(expected)
	
	return actual == expected
}

// NormalizeString 规范化字符串，去除空白并统一换行符
// 导出此函数以便可在其他包中使用（如客户端显示比较结果）
func NormalizeString(s string) string {
	// 统一换行符
	s = regexp.MustCompile(`\r\n|\r`).ReplaceAllString(s, "\n")
	// 去除行首行尾空白
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	// 重新组合，去除末尾空行
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// LookPath is a wrapper around exec.LookPath
func LookPath(file string) (string, error) {
    return exec.LookPath(file)
}