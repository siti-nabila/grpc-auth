APP_NAME=grpc-auth-service
BIN_DIR=bin

.PHONY: all build run clean

all: clean build run

build:
	@echo "🚧 Building $(APP_NAME)..."
	go build -o $(BIN_DIR)/$(APP_NAME) cmd/main.go

run:
	@echo "🚀 Running $(APP_NAME)..."
	./$(BIN_DIR)/$(APP_NAME)

clean:
	@echo "🧹 Cleaning build..."
	rm -f $(BIN_DIR)/$(APP_NAME)
