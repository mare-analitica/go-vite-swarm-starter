COMPOSE := docker compose -f compose.dev.yaml

.DEFAULT_GOAL := help

.PHONY: help dev web down clean logs test lint build

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "} /^[a-z-]+:.*## / {printf "  %-8s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

dev: ## Start PostgreSQL, MinIO, n8n and the API (http://localhost:8080)
	$(COMPOSE) up -d --build --wait
	@echo "API      http://localhost:8080/readyz"
	@echo "MinIO    http://localhost:9001 (dev-minio-root / dev-minio-root-password)"
	@echo "n8n      http://localhost:5678"
	@echo "Frontend: make web"

web: ## Run the Vite dev server (http://localhost:5173)
	cd frontend && npm ci && npm run dev

down: ## Stop the environment (keeps data)
	$(COMPOSE) down

clean: ## Stop the environment and delete its data
	$(COMPOSE) down --volumes

logs: ## Follow API logs
	$(COMPOSE) logs -f api

test: ## Run backend tests and frontend build
	cd backend && go test ./...
	cd frontend && npm ci && npm run build

lint: ## Lint backend and frontend
	cd backend && go vet ./... && gofmt -l . | (! grep .)
	cd frontend && npm ci && npm run lint

build: ## Build production images locally
	docker build -t starter-api:local backend
	docker build -t starter-web:local frontend
