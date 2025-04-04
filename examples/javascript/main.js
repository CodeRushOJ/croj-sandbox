const readline = require('readline').createInterface({
  input: process.stdin,
  output: process.stdout
});

function fibonacci(n) {
  if (n <= 1) return n;
  
  let a = 0, b = 1;
  for (let i = 2; i <= n; i++) {
    let temp = a + b;
    a = b;
    b = temp;
  }
  return b;
}

// 注册事件处理器读取输入
readline.question('', (input) => {
  const n = parseInt(input);
  
  for (let i = 0; i < n; i++) {
    console.log(`${fibonacci(i)}`);
  }
  
  readline.close();
});
