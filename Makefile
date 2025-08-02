# ctxtrans Makefile

BINARY_NAME=ctxtrans
VERSION=1.0.0
BUILD_TIME=$(shell date +%Y-%m-%d_%H:%M:%S)
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.GitCommit=${GIT_COMMIT}"

# 默认目标
.PHONY: all
all: build

# 构建二进制文件
.PHONY: build
build:
	@echo "Building ${BINARY_NAME}..."
	@go build ${LDFLAGS} -o ${BINARY_NAME} ./cmd/ctxtrans

# 安装到 $GOPATH/bin
.PHONY: install
install:
	@echo "Installing ${BINARY_NAME}..."
	@go install ${LDFLAGS} ./cmd/ctxtrans

# 清理构建文件
.PHONY: clean
clean:
	@echo "Cleaning..."
	@rm -f ${BINARY_NAME}
	@go clean

# 运行测试
.PHONY: test
test:
	@echo "Running tests..."
	@go test ./internal/ctxtrans/...

# 运行测试并显示覆盖率
.PHONY: test-coverage
test-coverage:
	@echo "Running tests with coverage..."
	@go test -cover ./internal/ctxtrans/...

# 格式化代码
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# 静态检查
.PHONY: lint
lint:
	@echo "Running linter..."
	@golangci-lint run

# 检查模块依赖
.PHONY: deps
deps:
	@echo "Checking dependencies..."
	@go mod tidy
	@go mod verify

# 运行示例
.PHONY: example
example:
	@echo "Running example..."
	@OPENAI_API_KEY=demo-key ./ctxtrans -help

# 帮助信息
.PHONY: help
help:
	@echo "Available targets:"
	@echo "  build        - Build the binary"
	@echo "  install      - Install to \$$GOPATH/bin"
	@echo "  test         - Run tests"
	@echo "  test-coverage- Run tests with coverage"
	@echo "  fmt          - Format code"
	@echo "  lint         - Run linter"
	@echo "  clean        - Clean build artifacts"
	@echo "  deps         - Check dependencies"
	@echo "  example      - Run example"
	@echo "  help         - Show this help"

