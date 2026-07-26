SHELL := bash
FRONTEND := frontend
GO_PKGS = $(shell go list ./... | grep -v '/frontend/node_modules/')

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-16s %s\n", $$1, $$2}'

.PHONY: install
install: ## Install frontend dependencies without lifecycle scripts
	cd $(FRONTEND) && npm ci --ignore-scripts --no-audit --no-fund

.PHONY: fmt
fmt: ## Format Go source
	gofmt -w $$(find . -name '*.go' -not -path './frontend/node_modules/*')

.PHONY: fmt-check
fmt-check: ## Fail when Go source is not formatted
	@test -z "$$(gofmt -l $$(find . -name '*.go' -not -path './frontend/node_modules/*'))"

.PHONY: lint-go
lint-go: ## Run Go static analysis
	go vet $(GO_PKGS)

.PHONY: lint-fe
lint-fe: ## Run frontend ESLint
	cd $(FRONTEND) && npm run lint

.PHONY: lint
lint: lint-go lint-fe ## Run all linters

.PHONY: typecheck
typecheck: ## Type-check the frontend
	cd $(FRONTEND) && npm run typecheck

.PHONY: test-go
test-go: ## Run all Go tests
	go test -shuffle=on -count=1 $(GO_PKGS)

.PHONY: test-fe
test-fe: ## Run frontend unit tests
	cd $(FRONTEND) && npm test

.PHONY: test
test: test-go test-fe ## Run all tests

.PHONY: build-fe
build-fe: ## Build the embedded frontend
	cd $(FRONTEND) && npm run build

.PHONY: build-go
build-go: build-fe ## Build the panel binary
	go build $(GO_PKGS)

.PHONY: build-storybook
build-storybook: ## Compile all Storybook stories
	cd $(FRONTEND) && npm run build-storybook

.PHONY: quadlet-check
quadlet-check: ## Generate and validate the Quadlet with the installed Podman
	@sh deploy/quadlet/verify.sh

.PHONY: image
image: ## Build the linux/amd64 OCI image with Podman
	podman build --network host --platform linux/amd64 --format oci -f Containerfile -t localhost/phantom:verify .

.PHONY: verify
verify: fmt-check lint typecheck test build-go build-storybook quadlet-check ## Run the complete local gate
	@echo "verify: OK"
