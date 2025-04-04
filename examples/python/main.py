def calculate(expr):
    return int(eval(expr))

def main():
    
    n = int(input().strip())
    
    for _ in range(n):
        expr = input().strip()
        result = calculate(expr)
        print(f"{result}")

if __name__ == "__main__":
    main()
