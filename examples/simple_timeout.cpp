#include <iostream>
#include <chrono>
#include <thread>

// 一个非常简单的程序，运行固定时间
int main(int argc, char* argv[]) {
    int seconds = 10;  // 默认运行10秒
    
    if (argc > 1) {
        seconds = atoi(argv[1]);
    }
    
    std::cout << "将要运行 " << seconds << " 秒..." << std::endl;
    
    for (int i = 1; i <= seconds; ++i) {
        std::cout << "已经运行 " << i << " 秒" << std::endl;
        std::cout.flush();
        std::this_thread::sleep_for(std::chrono::seconds(1));
    }
    
    std::cout << "成功运行完成!" << std::endl;
    return 0;
}
