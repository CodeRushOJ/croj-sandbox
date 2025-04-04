#include <iostream>
#include <vector>
#include <string>

// 测试内存限制的程序，尝试分配大量内存
int main() {
    int n;
    std::cin >> n; // 从输入读取要分配的MB数
    
    std::cout << "尝试分配 " << n << " MB 内存..." << std::endl;
    
    try {
        // 每个vector存储1MB数据
        std::vector<std::vector<char>> memory;
        
        for (int i = 0; i < n; i++) {
            // 分配1MB (1024 * 1024 字节)
            std::vector<char> mb(1024 * 1024, 'X');
            memory.push_back(mb);
            
            if (i % 10 == 0) {
                std::cout << "已分配 " << i << " MB" << std::endl;
            }
        }
        
        std::cout << "成功分配 " << n << " MB 内存" << std::endl;
    } catch (const std::bad_alloc& e) {
        std::cout << "内存分配失败: " << e.what() << std::endl;
        return 1;
    }
    
    return 0;
}
