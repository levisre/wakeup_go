BINARY_NAME=wakebot_go
GO_FILES=$(shell find . -name '*.go')

.PHONY: all build clean run test help release

all: build

build:
	go build -o $(BINARY_NAME)

release:
	@echo "Building for release..."
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-s -w --extldflags "-static"' -o $(BINARY_NAME) .
	@echo "Build complete: $(BINARY_NAME)"

run: build
	./$(BINARY_NAME)

test:
	go test -v ./...

clean:
	go clean
	rm -f $(BINARY_NAME)

help:
	@echo "Usage:"
	@echo "  make build   - Build the binary"
	@echo "  make release - Build a statically linked and stripped binary for release"
	@echo "  make run     - Build and run the application"
	@echo "  make test    - Run tests"
	@echo "  make clean   - Remove binary and build artifacts"

