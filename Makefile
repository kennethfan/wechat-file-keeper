# wechat-file-keeper — macOS 微信文件管理工具
#
# 说明：本机 Go 模块缓存在 ~/.gvm，受限沙箱中构建需把缓存指到可写位置。

BINARY  := wechat-file-keeper
BUILD   := bin
PKG     := ./cmd/wechat-file-keeper

# 受限环境下用工作区内的缓存目录，避免写 ~/.gvm 被拒。
export GOPATH     ?= $(CURDIR)/.gopath
export GOMODCACHE ?= $(CURDIR)/.gomodcache
export GOCACHE    ?= $(CURDIR)/.gocache

.PHONY: all build scan test cover vet fmt check clean help

all: check build

## build: 编译二进制到 bin/
build:
	@mkdir -p $(BUILD)
	go build -trimpath -ldflags "-s -w" -o $(BUILD)/$(BINARY) $(PKG)
	@echo "已生成 $(BUILD)/$(BINARY)"

## scan: 编译并对本机微信账号库做只读扫描
scan: build
	./$(BUILD)/$(BINARY) scan

## test: 运行全部测试
test:
	go test ./...

## cover: 运行测试并输出覆盖率
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

## vet: 静态检查
vet:
	go vet ./...

## fmt: 格式化代码
fmt:
	gofmt -w .

## check: 格式、静态检查、测试一次跑完
check:
	@test -z "$$(gofmt -l .)" || { echo "以下文件需要 gofmt:"; gofmt -l .; exit 1; }
	go vet ./...
	go test ./...

## clean: 清理构建与缓存产物
clean:
	rm -rf $(BUILD) coverage.out .gopath .gomodcache .gocache

## help: 显示本帮助
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
