package main

import "fmt"

func main() {
	// input a, b from command line
	var a, b int
	fmt.Scanf("%d %d", &a, &b)

	// output a + b
	fmt.Println("a + b = ", a+b)
}