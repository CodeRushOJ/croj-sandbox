# croj-sandbox 示例代码

这个目录包含了各种编程语言的示例代码、输入文件和期望输出文件，用于测试 croj-sandbox 的功能。

## 使用方法

使用 simple-client 工具运行示例代码并比较输出：

```bash
# 编译客户端工具
cd /Users/zfhe/mywork/croj/croj-sandbox
go build -o simple-client cmd/simple-client/main.go

# 运行 Go 示例并比较输出
./simple-client -source examples/go/main.go -stdin examples/go/sum_input.txt -output examples/go/sum_output.txt

# 运行 C++ 示例并比较输出
./simple-client -source examples/cpp/main.cpp -stdin examples/cpp/sort_input.txt -output examples/cpp/sort_output.txt

# 运行 Python 示例并比较输出
./simple-client -source examples/python/main.py -stdin examples/python/calculator_input.txt -output examples/python/calculator_output.txt

# 运行 Java 示例并比较输出
./simple-client -source examples/java/Main.java -stdin examples/java/wordcount_input.txt -output examples/java/wordcount_output.txt

# 运行 JavaScript 示例并比较输出
./simple-client -source examples/javascript/main.js -stdin examples/javascript/fibonacci_input.txt -output examples/javascript/fibonacci_output.txt
```

## 示例说明

每个语言目录中包含以下类型的文件：

1. 测试程序，需要输入和输出比较
2. 输入文件 (.txt)，包含测试数据
3. 期望输出文件 (.txt)，包含程序运行的正确输出结果

## 调试提示

如果输出比较失败，使用详细模式查看更多细节：

```bash
./simple-client -v -source examples/go/sum.go -stdin examples/go/sum_input.txt -output examples/go/sum_output.txt
```

这将显示规范化后的字符串以便更容易发现空白符、换行符等差异。
