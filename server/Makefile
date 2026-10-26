# 运维专家服务平台 · 后端
# Windows 下若没有 make，直接看 README 里的等价命令即可。

APP_NAME := ops-expert
BIN_DIR  := bin

# Windows 下 go build -o 不会给产物自动补 .exe（它只对默认输出名做这件事），
# 少了这个后缀，bin/api 在 cmd / PowerShell 里直接执行会报"找不到文件"。
# 冒烟脚本两个名字都会找，所以这里保持一致即可。
ifeq ($(OS),Windows_NT)
API_EXT := .exe
endif
API_BIN  := $(BIN_DIR)/api$(API_EXT)

VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)

# 部署自检的目标地址：make verify-deploy BASE=http://172.28.68.55:8080
BASE     ?= http://127.0.0.1:8080

.PHONY: help
help: ## 显示所有可用命令
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: run
run: ## 本地启动（零依赖，SQLite + 内存缓存）
	go run ./cmd/api

.PHONY: run-mysql
run-mysql: ## 用 MySQL + Redis 启动（需先 up 起依赖）
	DB_DRIVER=mysql \
	DB_DSN='root:root@tcp(127.0.0.1:3306)/ops_expert?charset=utf8mb4&parseTime=True&loc=Local' \
	REDIS_ADDR=127.0.0.1:6379 \
	go run ./cmd/api

.PHONY: build
build: ## 编译二进制到 bin/
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(API_BIN) ./cmd/api
	@echo "已生成 $(API_BIN)（版本 $(VERSION)）"

.PHONY: build-linux
build-linux: ## 交叉编译 Linux amd64 二进制
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="$(LDFLAGS)" -o $(API_BIN)-linux-amd64 ./cmd/api

.PHONY: tidy
tidy: ## 整理依赖
	go mod tidy

.PHONY: test
test: ## 运行单元测试
	go test ./... -count=1

.PHONY: test-cover
test-cover: ## 运行测试并输出覆盖率
	go test ./... -count=1 -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

.PHONY: vet
vet: ## 静态检查
	go vet ./...

.PHONY: fmt
fmt: ## 格式化
	gofmt -s -w .

.PHONY: check
check: fmt vet test ## 格式化 + 检查 + 测试

.PHONY: clean
clean: ## 清理构建产物与本地 SQLite 数据
	rm -rf $(BIN_DIR) coverage.out data/

# 原 up/down 指向 deploy/docker-compose.yml，这个文件从来没存在过；
# deploy/ 现在是服务器部署物（install.sh + systemd 单元）的目录，别在这里放容器编排。
.PHONY: deploy-binary
deploy-binary: ## 交叉编译出可直接上传的 Linux 二进制（dist/ops-expert-api）
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
		-ldflags "-s -w -X main.version=$(VERSION)" -o dist/ops-expert-api ./cmd/api
	@ls -lh dist/ops-expert-api

.PHONY: verify-deploy
verify-deploy: ## 对已在运行的实例做部署自检（BASE=http://host:8080）
	python scripts/verify_deploy.py $(BASE)

.PHONY: smoke
smoke: build ## 端到端冒烟：知识库 + 订单（每个套件各起一个全新库与服务进程）
	bash scripts/smoke.sh

.PHONY: smoke-twice
smoke-twice: build ## 连跑两趟冒烟，比对断言总数是否稳定（发现"某条断言这次没跑"的唯一办法）
	bash scripts/smoke_twice.sh
