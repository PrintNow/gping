# 本地产物目录（默认与 build.sh 中的项目内构建一致）
BIN_DIR ?= bin
OUTPUT := $(BIN_DIR)/gping

.PHONY: build clean

build:
	mkdir -p $(BIN_DIR)
	go build -o $(OUTPUT) .

clean:
	rm -f $(OUTPUT)
