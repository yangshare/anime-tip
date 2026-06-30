# anime-tip Makefile
# 统一本地构建入口：编译产物固定输出到 build/，避免污染项目根目录。
#
# 常用命令：
#   make            # 等同 make build
#   make build      # 编译到 build/anime-tip(.exe)
#   make run        # 直接 go run
#   make clean      # 清理 build/ 与运行时日志
#
# 说明：Windows 下需安装 Git for Windows（自带 make）或 MSYS2。

# 二进制名：Windows 自动加 .exe 后缀
ifeq ($(OS),Windows_NT)
	BIN_NAME := anime-tip.exe
else
	BIN_NAME := anime-tip
endif

BUILD_DIR := build
BIN := $(BUILD_DIR)/$(BIN_NAME)
PKG      := ./cmd/server/

# 默认目标
.PHONY: all
all: build

# 编译产物到 build/
# go build -o 会自动创建输出目录，无需手动 mkdir（且兼容 Windows/Unix）
.PHONY: build
build:
	@go build -o $(BIN) $(PKG)
	@echo "已编译: $(BIN)"

# 直接运行（不产出二进制）
.PHONY: run
run:
	go run $(PKG)

# 清理编译产物与运行时日志
# Windows 默认 shell 为 cmd.exe，没有 rm/mkdir -p，故显式调用 PowerShell。
.PHONY: clean
clean:
ifeq ($(OS),Windows_NT)
	@powershell -NoProfile -Command "Remove-Item -Recurse -Force '$(BUILD_DIR)' -ErrorAction SilentlyContinue; Remove-Item -Force -ErrorAction SilentlyContinue server.log,stdout.log,stderr.log; exit 0"
else
	@rm -rf $(BUILD_DIR)
	@rm -f server.log stdout.log stderr.log
endif
	@echo "已清理 $(BUILD_DIR)/ 与运行时日志"
