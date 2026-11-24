.PHONY: run build ui test test-redis smoke docker up down fmt

run:            ## run locally (in-memory store, no dependencies)
	cd backend && go run ./cmd/server

build: ## build the binary (memory/redis only; Docker build adds PostgreSQL)
	cd backend && go build -o ../bin/mock-digilocker ./cmd/server

ui:             ## rebuild the React UI into backend/internal/webui/dist
	cd frontend && (test -d node_modules || npm install) && npm run build

test:           ## Go + UI unit tests
	cd backend && go test ./...
	cd frontend && npm test

test-redis:     ## Go tests against a local Redis (localhost:6379)
	cd backend && TEST_REDIS_ADDR=localhost:6379 go test -count=1 ./...

smoke:          ## headless end-to-end check against BASE_URL (default http://localhost:8080)
	./scripts/smoke-test.sh

up:             ## docker compose stack (PostgreSQL + Redis)
	docker compose up --build -d && echo "→ http://localhost:8080"

down:
	docker compose down

fmt:
	cd backend && gofmt -w .
