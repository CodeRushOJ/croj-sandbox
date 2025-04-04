package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	
	// 读取数字个数
	scanner.Scan()
	n, _ := strconv.Atoi(scanner.Text())
	
	// 读取 n 个数字并计算总和
	sum := 0
	for i := 0; i < n; i++ {
		scanner.Scan()
		num, _ := strconv.Atoi(scanner.Text())
		sum += num
	}
	
	fmt.Println(sum)
}
