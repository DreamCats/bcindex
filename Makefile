.PHONY: all build test clean install help fmt lint vet

# 默认目标
all: build

# 构建 bcindex
build:
	@echo "Building bcindex..."
	go build -o bcindex ./cmd/bcindex
	@echo "✅ Build complete: ./bcindex"

# 安装到 PATH
install: build
	@echo "Installing bcindex to /usr/local/bin..."
	sudo mv bcindex /usr/local/bin/
	@echo "✅ Installed to /usr/local/bin/bcindex"

# 运行测试
test:
	@echo "Running tests..."
	go test -v ./...

# 清理构建产物
clean:
	@echo "Cleaning..."
	rm -f bcindex
	rm -rf ~/.bcindex/data/*
	@echo "✅ Clean complete"

# 格式化代码
fmt:
	@echo "Formatting code..."
	go fmt ./...
	@echo "✅ Format complete"

# 静态检查
vet:
	@echo "Running go vet..."
	go vet ./...
	@echo "✅ Vet complete"

# 运行 golangci-lint
lint:
	@echo "Running golangci-lint..."
	golangci-lint run
	@echo "✅ Lint complete"

# 显示帮助
help:
	@echo "BCIndex Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make build    - 构建 bcindex 二进制文件"
	@echo "  make install  - 安装到 /usr/local/bin"
	@echo "  make test     - 运行测试"
	@echo "  make clean    - 清理构建产物"
	@echo "  make fmt      - 格式化代码"
	@echo "  make vet      - 运行 go vet"
	@echo "  make lint     - 运行 golangci-lint"
	@echo "  make help     - 显示此帮助信息"
