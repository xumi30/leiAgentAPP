APP_NAME := proxy-lb
BIN_DIR := bin
CONFIG ?= config/config.yaml

.PHONY: run build tidy fmt clean

run:
	PROXY_LB_CONFIG=$(CONFIG) go run ./cmd/proxy-lb

build:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(APP_NAME) ./cmd/proxy-lb

tidy:
	go mod tidy

fmt:
	gofmt -w ./cmd ./internal

clean:
	rm -rf $(BIN_DIR)
