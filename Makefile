.PHONY: build run clean test vet fmt

BINARY := tron
BUILD_DIR := bin

build:
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/tron

run: build
	$(BUILD_DIR)/$(BINARY)

run-debug: build
	$(BUILD_DIR)/$(BINARY) -debug

clean:
	rm -rf $(BUILD_DIR)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	go fmt ./...

all: fmt vet test build
