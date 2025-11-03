APP_NAME=grpc-auth-service
BIN_DIR=bin
PROTO_SRC := $(shell find proto -name '*.proto')
PROTO_OUT := pb

all: build run

build:
	@echo "Building $(APP_NAME)..."
	go build -o $(BIN_DIR)/$(APP_NAME) cmd/api/main.go

run:
	@echo "Running $(APP_NAME)..."
	./$(BIN_DIR)/$(APP_NAME)

clean:
	@echo "Cleaning up..."
	rm -f $(BIN_DIR)/$(APP_NAME)

.PHONY: proto clean-proto
proto:
	@echo "🔧 Generating protobuf files..."
	@mkdir -p $(PROTO_OUT)
	@for file in $(PROTO_SRC); do \
		echo "⏳ Generating $$file..."; \
		protoc \
			--proto_path=proto \
			--go_out=$(PROTO_OUT) --go_opt=paths=source_relative \
			--go-grpc_out=$(PROTO_OUT) --go-grpc_opt=paths=source_relative \
			$$file || exit 1; \
	done
	@echo "✅ All .proto files generated successfully in $(PROTO_OUT)/"

clean-proto:
	@echo "🧹 Cleaning generated protobuf files..."
	@rm -rf $(PROTO_OUT)
