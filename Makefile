# ctxtrans Makefile

BINARY_NAME=ctxtrans
VERSION?=1.0.0
BUILD_TIME=$(shell date +%Y-%m-%d_%H:%M:%S)
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.GitCommit=${GIT_COMMIT}"
IMAGE_REGISTRY?=
IMAGE_NAME?=$(BINARY_NAME)
IMAGE_TAG?=$(VERSION)
IMAGE_FULL_NAME:=$(if $(IMAGE_REGISTRY),$(IMAGE_REGISTRY)/,)$(IMAGE_NAME)
PLATFORMS?=linux/amd64,linux/arm64
DOCKERFILE?=Dockerfile
DOCKER_BUILD_ARGS?=
BUILDX_BUILDER?=ctxtrans-builder

# 默认目标
.PHONY: all
all: build

# 构建二进制文件
.PHONY: build
build:
	@echo "Building ${BINARY_NAME}..."
	@go build ${LDFLAGS} -o ${BINARY_NAME} ./cmd

# 安装到 $GOPATH/bin
.PHONY: install
install:
	@echo "Installing ${BINARY_NAME}..."
	@go install ${LDFLAGS} ./cmd

# 运行应用
.PHONY: run
run: build
	@echo "Running ${BINARY_NAME}..."
	@UI_STATIC_DIR=$(CURDIR)/web/dist ./${BINARY_NAME}

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

# 构建 Docker 镜像
.PHONY: docker-build
docker-build:
	@echo "Building Docker image ${IMAGE_FULL_NAME}:${IMAGE_TAG}..."
	DOCKER_BUILDKIT=1 docker build -t ${IMAGE_FULL_NAME}:${IMAGE_TAG} -t ${IMAGE_FULL_NAME}:latest -f ${DOCKERFILE} ${DOCKER_BUILD_ARGS} .

# 构建并推送多架构镜像
.PHONY: ensure-buildx-builder
ensure-buildx-builder:
	@docker buildx inspect ${BUILDX_BUILDER} >/dev/null 2>&1 || docker buildx create --name ${BUILDX_BUILDER} --driver docker-container --use
	@docker buildx use ${BUILDX_BUILDER}

.PHONY: docker-build-multi
docker-build-multi: ensure-buildx-builder
	@echo "Building multi-arch image ${IMAGE_FULL_NAME}:${IMAGE_TAG} for platforms ${PLATFORMS}..."
	DOCKER_BUILDKIT=1 docker buildx build --builder ${BUILDX_BUILDER} --platform ${PLATFORMS} -t ${IMAGE_FULL_NAME}:${IMAGE_TAG} -t ${IMAGE_FULL_NAME}:latest -f ${DOCKERFILE} ${DOCKER_BUILD_ARGS} --push .

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
	@echo "  run          - Build and run the application"
	@echo "  install      - Install to \$$GOPATH/bin"
	@echo "  test         - Run tests"
	@echo "  test-coverage- Run tests with coverage"
	@echo "  fmt          - Format code"
	@echo "  lint         - Run linter"
	@echo "  clean        - Clean build artifacts"
	@echo "  deps         - Check dependencies"
	@echo "  docker-build - Build Docker image (local arch)"
	@echo "  ensure-buildx-builder - Create/select buildx builder (docker-container driver)"
	@echo "  docker-build-multi - Build and push multi-arch image with buildx"
	@echo "  example      - Run example"
	@echo "  help         - Show this help"
