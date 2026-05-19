APP_NAME  = seeder
BUILD_DIR = bin

.PHONY: all build install uninstall clean test test-integration lint

all: build

build:
	@echo "Building $(APP_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP_NAME) .

install:
	@echo "Installing $(APP_NAME)..."
	@bin_dir=$$(go env GOBIN); \
	if [ -z "$$bin_dir" ]; then \
		bin_dir=$$(go env GOPATH)/bin; \
	fi; \
	mkdir -p "$$bin_dir"; \
	echo "Installing to $$bin_dir/$(APP_NAME)"; \
	go build -o "$$bin_dir/$(APP_NAME)" .

uninstall:
	@echo "Uninstalling $(APP_NAME)..."
	@bin_dir=$$(go env GOBIN); \
	if [ -z "$$bin_dir" ]; then \
		bin_dir=$$(go env GOPATH)/bin; \
	fi; \
	echo "Removing $$bin_dir/$(APP_NAME)"; \
	rm -f "$$bin_dir/$(APP_NAME)"

clean:
	@echo "Cleaning up..."
	rm -rf $(BUILD_DIR)

test:
	go test ./... -race

test-integration:
	@if [ -z "$$SEEDER_TEST_DSN_MYSQL" ] && [ -z "$$SEEDER_TEST_DSN_POSTGRES" ]; then \
		echo "Set SEEDER_TEST_DSN_MYSQL and/or SEEDER_TEST_DSN_POSTGRES to run integration tests."; \
		echo "Examples:"; \
		echo "  SEEDER_TEST_DSN_MYSQL=mysql://root:pass@localhost:3306/dev?parseTime=true make test-integration"; \
		echo "  SEEDER_TEST_DSN_POSTGRES=postgres://postgres:pass@localhost:5432/dev?sslmode=disable make test-integration"; \
		echo "Tip: 'docker compose up -d' boots both databases."; \
		exit 1; \
	fi
	go test -tags=integration ./... -race -count=1 -p 1

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint is not installed"; \
		exit 1; \
	}
	golangci-lint run
