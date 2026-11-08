# Makefile — convenience targets. Run `make help` to list them.
# (Go projects often use a Makefile as the task runner, like npm scripts.)

.DEFAULT_GOAL := help

# ---- Phase 0/1: build, run, demo ------------------------------------------

.PHONY: tidy
tidy: ## Resolve and prune module dependencies
	go mod tidy

.PHONY: build
build: ## Compile all commands into ./bin
	go build -o bin/ ./cmd/...

.PHONY: gen-urls
gen-urls: ## Generate testdata/urls.txt (10k sample URLs)
	go run ./cmd/gen-urls -n 10000 -out testdata/urls.txt

.PHONY: testserver
testserver: ## Run the offline mock target server on :8088
	go run ./cmd/testserver

.PHONY: worker
worker: ## Run the health-check engine over testdata/urls.txt (race detector on)
	go run -race ./cmd/worker -file testdata/urls.txt -workers 50 -timeout 3s

.PHONY: demo
demo: ## One-shot demo: start mock server, generate URLs, run the engine
	@echo ">> starting mock server in background"; \
	go run ./cmd/testserver & SRV=$$!; \
	sleep 1; \
	$(MAKE) gen-urls; \
	go run -race ./cmd/worker -file testdata/urls.txt -workers 50 -timeout 3s; \
	kill $$SRV

# ---- Learning track -------------------------------------------------------

.PHONY: lesson
lesson: ## Run a learn/ lesson by number, e.g. `make lesson L=04` (race detector on)
	@test -n "$(L)" || { echo "usage: make lesson L=<nn>   (e.g. make lesson L=04)"; exit 2; }
	@dir=$$(ls -d learn/$(L)-*/ 2>/dev/null | head -1); \
	test -n "$$dir" || { echo "no lesson matching learn/$(L)-*"; ls -d learn/*/; exit 2; }; \
	echo ">> go run -race ./$$dir"; go run -race ./$$dir

# ---- Quality --------------------------------------------------------------

.PHONY: test
test: ## Run all tests with the race detector
	go test -race ./...

.PHONY: cover
cover: ## Run tests and open an HTML coverage report
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

.PHONY: vet
vet: ## Run go vet static checks
	go vet ./...

.PHONY: fmt
fmt: ## Format all code
	gofmt -l -w .

.PHONY: lint
lint: ## Run golangci-lint (install: https://golangci-lint.run)
	golangci-lint run

# ---- Help -----------------------------------------------------------------

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
