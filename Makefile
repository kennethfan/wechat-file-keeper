# wechat-file-keeper — macOS 微信文件管理工具
#
# 说明：本机 Go 模块缓存在 ~/.gvm，受限沙箱中构建需把缓存指到可写位置。

BINARY  := wechat-file-keeper
BUILD   := bin
PKG     := .
DIST    := dist
APP     := $(DIST)/微信文件管家.app
VERSION := 0.1.0

# 正式签名用的开发者身份。留空则使用 ad-hoc 签名（够本机运行）。
# 用法：make app SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)"
SIGN_IDENTITY ?=

# 受限环境下用工作区内的缓存目录，避免写 ~/.gvm 被拒。
export GOPATH     ?= $(CURDIR)/.gopath
export GOMODCACHE ?= $(CURDIR)/.gomodcache
export GOCACHE    ?= $(CURDIR)/.gocache

.PHONY: all build scan test cover vet fmt check clean help \
        app icon

all: check build

## build: 编译二进制到 bin/
build:
	@mkdir -p $(BUILD)
	go build -trimpath -ldflags "-s -w" -o $(BUILD)/$(BINARY) $(PKG)
	@echo "已生成 $(BUILD)/$(BINARY)"

## app: 打包 dist/微信文件管家.app（含图标与 ad-hoc 签名）
app: build icon
	@rm -rf "$(APP)"
	@mkdir -p "$(APP)/Contents/MacOS" "$(APP)/Contents/Resources"
	@cp $(BUILD)/$(BINARY) "$(APP)/Contents/MacOS/$(BINARY)"
	@cp packaging/Info.plist "$(APP)/Contents/Info.plist"
	@cp packaging/AppIcon.icns "$(APP)/Contents/Resources/AppIcon.icns"
	@sed -i '' 's|<string>0\.1\.0</string>|<string>$(VERSION)</string>|g' "$(APP)/Contents/Info.plist"
	@if [ -n "$(SIGN_IDENTITY)" ]; then \
		codesign --force --deep --options runtime \
			--sign "$(SIGN_IDENTITY)" "$(APP)"; \
		echo "已用 $(SIGN_IDENTITY) 签名"; \
	else \
		codesign --force --deep --sign - "$(APP)"; \
		echo "已用 ad-hoc 签名（仅本机可用）"; \
	fi
	@echo "已生成 $(APP)"

## icon: 由 packaging/icon-source.png 重新生成 AppIcon.icns
icon:
	@python3 packaging/make_icon_source.py
	@python3 packaging/make_icon.py

## scan: 编译并对本机微信账号库做只读扫描
scan: build
	./$(BUILD)/$(BINARY) scan

## gui: 编译并启动图形界面
gui: build
	./$(BUILD)/$(BINARY) gui

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
	gofmt -w cmd internal

## check: 格式、静态检查、测试一次跑完
check:
	@test -z "$$(gofmt -l cmd internal)" || { echo "以下文件需要 gofmt:"; gofmt -l cmd internal; exit 1; }
	go vet ./...
	go test ./...

## clean: 清理构建与缓存产物
clean:
	rm -rf $(BUILD) $(DIST) coverage.out .gopath .gomodcache .gocache

## help: 显示本帮助
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  /'
