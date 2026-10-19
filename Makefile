BINARY := bin/netcfg
WEB_DIR := web
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: setup build build-web build-go run test test-go test-web lint fmt help

help: ## 显示帮助
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

setup: ## 安装前端依赖
	cd $(WEB_DIR) && npm install

build-web: ## 构建前端（产物 web/dist）
	cd $(WEB_DIR) && npm run build

build-go: ## 编译后端单二进制（内嵌前端）
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

build: build-web build-go ## 构建前端 + 后端

run: build-go ## 运行 CLI
	$(BINARY) list

serve: build ## 启动 Web UI（默认 http://127.0.0.1:8190）
	$(BINARY) serve

test-go: ## Go 单元测试
	go test ./...

test-web: ## 前端构建校验
	cd $(WEB_DIR) && npm run build

test: test-go test-web ## 全量测试

lint: ## go vet
	go vet ./...

fmt: ## gofmt 检查并修复
	gofmt -w .

clean: ## 清理构建产物
	rm -rf bin $(WEB_DIR)/dist
