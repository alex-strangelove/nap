GO ?= go
BINARY ?= nap
BUILD_DIR ?= bin
APT_PACKAGES ?= golang-go

.PHONY: all apt-deps bootstrap build test clean

all: build

apt-deps:
	sudo apt-get update
	sudo apt-get install -y $(APT_PACKAGES)

bootstrap: apt-deps build

build:
	mkdir -p $(BUILD_DIR)
	$(GO) build -o $(BUILD_DIR)/$(BINARY) ./cmd/nap

test:
	$(GO) test ./...

clean:
	rm -f $(BUILD_DIR)/$(BINARY)
