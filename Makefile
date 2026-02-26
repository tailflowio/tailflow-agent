.PHONY: build test clean frontend dev server run validate help gen-mocks

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BINARY := tailflow
GOFLAGS := -ldflags "-X main.version=$(VERSION)"

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: frontend ## Build the binary with embedded UI
	go build $(GOFLAGS) -o bin/$(BINARY) ./cmd/tailflow/

build-quick: ## Build without frontend (faster)
	go build $(GOFLAGS) -o bin/$(BINARY) ./cmd/tailflow/

GO := go
COVER_OUT := coverage.out
PKGS_WITH_TESTS := $(shell $(GO) list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' ./...)

test: ## Run all Go tests
	$(GO) test ./... -v

test-coverage: ## Run tests with coverage (100% required, mocks excluded)
	@$(GO) test $(PKGS_WITH_TESTS) -coverprofile=$(COVER_OUT) -covermode=count > /dev/null
	@coverage=$$($(GO) tool cover -func=$(COVER_OUT) | grep total | awk '{print $$3}' | sed 's/%//'); \
	echo "coverage: $$coverage% of statements"; \
	if [ "$$coverage" != "100.0" ]; then \
		echo "Error: global coverage is $$coverage%, it should be 100.0%."; \
		exit 1; \
	fi

frontend: ## Build the VueJS frontend
	cd web/frontend && npm install && npx vite build

frontend-dev: ## Start frontend dev server
	cd web/frontend && npm install && npx vite

dev: build-quick ## Build and run server in dev mode
	./bin/$(BINARY) server --dir examples

server: build ## Build and run production server
	./bin/$(BINARY) server --dir examples

run: build-quick ## Run an example workflow
	./bin/$(BINARY) run examples/hello.yaml

validate: build-quick ## Validate example workflows
	@for f in examples/*.yaml; do \
		echo "Validating $$f..."; \
		./bin/$(BINARY) validate $$f; \
	done

clean: ## Remove build artifacts
	rm -rf bin/ web/dist/ coverage.out

docker-build: ## Build Docker image
	docker build --build-arg VERSION=$(VERSION) -f build/package/Dockerfile -t tailflow:$(VERSION) .

lint: ## Run Go linter
	golangci-lint run --config scripts/.golangci.yaml

gen-mocks: ## Regenerate mock implementations with mockery
	@command -v mockery >/dev/null 2>&1 || { echo "mockery not found. Install: go install github.com/vektra/mockery/v2@latest"; exit 1; }
	GOFLAGS= mockery --config scripts/.mockery.yaml

tidy: ## Tidy Go modules
	go mod tidy
