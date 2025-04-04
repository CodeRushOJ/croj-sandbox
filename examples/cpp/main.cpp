#include <iostream>
#include <vector>
#include <algorithm>
#include <string>

int main() {
    int n;
    std::cin >> n;
    
    std::vector<int> numbers(n);
    for (int i = 0; i < n; ++i) {
        std::cin >> numbers[i];
    }
    
    std::sort(numbers.begin(), numbers.end());
    
    for (int i = 0; i < n; ++i) {
        std::cout << numbers[i] << " ";
    }
    std::cout << std::endl;
    
    return 0;
}
