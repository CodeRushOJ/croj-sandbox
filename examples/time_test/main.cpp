#include <iostream>
#include <chrono>
#include <thread>
#include <iomanip>

// 更精确的时间限制测试
int main() {
    std::cout << "开始执行超时测试..." << std::endl;
    std::cout.flush();
    
    auto start_time = std::chrono::steady_clock::now();
    
    // 每100ms报告一次进度
    for (int i = 1; i <= 100; ++i) {
        auto current_time = std::chrono::steady_clock::now();
        auto elapsed = std::chrono::duration<double>(current_time - start_time).count();
        
        if (i % 10 == 0) {
            std::cout << std::fixed << std::setprecision(2);
            std::cout << "已运行 " << elapsed << " 秒" << std::endl;
            std::cout.flush();
        }
        
        // 在接近1秒时打印特殊消息
        if (elapsed >= 0.9 && elapsed < 1.0) {
            std::cout << ">>> 如果超时设置为1秒，程序即将被终止... <<<" << std::endl;
            std::cout.flush();
        }
        
        std::this_thread::sleep_for(std::chrono::milliseconds(100));
    }
    
    // 如果运行到这里，说明没有被正确终止
    auto end_time = std::chrono::steady_clock::now();
    auto total_time = std::chrono::duration<double>(end_time - start_time).count();
    
    std::cout << "程序完成! 总运行时间: " << total_time << " 秒" << std::endl;
    std::cout << "错误: 超时限制未生效!" << std::endl;
    
    return 0;
}