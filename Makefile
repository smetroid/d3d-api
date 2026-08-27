SHELL := /bin/bash
POSTGRES_CONTAINER=pg
POSTGRES_DSN?=postgres://postgres:postgres@localhost:5432/samus?sslmode=disable
RETHINKDB_CONTAINER=rethinkdb

.PHONY: postgres-start postgres-stop start-api-service install-gin-autoreload go-requirements \
	run-debug kill-debug migrate-from-rethinkdb lint test test-postgres build check \
	precommit-install precommit-run

postgres-start:
	docker start $(POSTGRES_CONTAINER) 2>/dev/null || docker run -d --name $(POSTGRES_CONTAINER) -p 5432:5432 -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=samus postgres:16-alpine

postgres-stop:
	docker stop $(POSTGRES_CONTAINER)

start-api-service:
	$(MAKE) postgres-start
	gin -i --all --bin .gin-bin run --config samus_dev.toml

install-gin-autoreload:
	go install github.com/codegangsta/gin@latest

# 1.16.4 golang upgrade
go-requirements:
	go mod init # install modules based on glide
	go mod tidy # add missing or remove modules


#run-debug: kill-debug
run-debug:
	/usr/local/go/bin/go build -gcflags="all=-N -l" samus.go
	./samus & dlv attach $$(echo "$$!") \
		--listen=:2345 \
		--headless=true \
		--log=true \
		--log-output=debugger,debuglineerr,gdbwire,lldbout,rpc \
		--accept-multiclient \
		--api-version=2


kill-debug:
	$(eval ID:=$(shell cat /tmp/samus.id))
	@if [ -n "${ID}" ]; then kill -9 ${ID}; else echo "samus.id not found"; fi
	$(eval DLV:=$(shell cat /tmp/dlv.id 2>/dev/null))
	@if [ -n "${DLV}" ]; then kill -9 ${DLV}; else echo "dlv.id not found"; fi

# CI/quality checks
lint:
	golangci-lint run --timeout 5m

test:
	go test -race -count=1 ./...

test-postgres:
	TEST_DATABASE_URL="$(POSTGRES_DSN)" go test -race -count=1 ./app/db/postgres/...

build:
	CGO_ENABLED=0 go build -o main .

check: lint test build

precommit-install:
	pre-commit install

precommit-run:
	pre-commit run --all-files

# Copy data from a running RethinkDB into Postgres (idempotent; see README).
migrate-from-rethinkdb:
	go run ./cmd/migrate-rb2pg \
		-rb-address localhost:28015 \
		-rb-database samus \
		-pg-dsn "$(POSTGRES_DSN)"
