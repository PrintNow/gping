# 本地产物目录（默认与 build.sh 中的仓库内构建一致）
BIN_DIR ?= bin
OUTPUT := $(BIN_DIR)/gping

# GeoLite2 City（.mmdb），与 geoip 默认查找路径 data/GeoLite2-City.mmdb 一致
DATA_DIR ?= data
MMDB := $(DATA_DIR)/GeoLite2-City.mmdb
# 浏览器里看到的是 blob 页；实际下载用 raw 内容
GEOLITE_GZ_URL ?= https://raw.githubusercontent.com/wp-statistics/GeoLite2-City/master/GeoLite2-City.mmdb.gz

.PHONY: build clean download-geolite geolite clean-geolite

build:
	mkdir -p $(BIN_DIR)
	go build -o $(OUTPUT) .

clean:
	rm -f $(OUTPUT)

# 下载 gzip 并在 data/ 下解压为 GeoLite2-City.mmdb（需本机有 curl、gunzip）
download-geolite geolite:
	mkdir -p $(DATA_DIR)
	curl -fL '$(GEOLITE_GZ_URL)' -o '$(DATA_DIR)/GeoLite2-City.mmdb.gz'
	gunzip -f '$(DATA_DIR)/GeoLite2-City.mmdb.gz'
	@echo "Wrote $(MMDB)"

clean-geolite:
	rm -f '$(MMDB)' '$(DATA_DIR)/GeoLite2-City.mmdb.gz'
