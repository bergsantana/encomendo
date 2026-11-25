GO ?= go
GOTEST_FLAGS ?= -race -shuffle=on -count=1
COVERPKG := ./internal/...

.PHONY: build test test-integration coverage lint fmt run migrate-up migrate-down migrate-reset migrate-status tidy

build:
	$(GO) build ./...

test:
	$(GO) test $(GOTEST_FLAGS) -coverpkg=$(COVERPKG) -coverprofile=unit.out ./internal/... ./cmd/... ./test/contract/...

test-integration:
	$(GO) test -race -count=1 -tags integration -coverpkg=$(COVERPKG) -coverprofile=integration.out ./test/integration/... ./internal/adapter/postgres/...

coverage: test
	./scripts/coverage-gate.sh unit.out $(wildcard integration.out)

lint:
	golangci-lint run ./...

fmt:
	golangci-lint fmt ./...

run:
	$(GO) run ./cmd/api

migrate-up:
	$(GO) run ./cmd/migrate up

migrate-down:
	$(GO) run ./cmd/migrate down

migrate-reset:
	$(GO) run ./cmd/migrate reset

migrate-status:
	$(GO) run ./cmd/migrate status

tidy:
	$(GO) mod tidy
