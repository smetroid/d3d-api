SHELL := /bin/bash
POSTGRES_CONTAINER=pg
POSTGRES_DSN?=postgres://postgres:postgres@localhost:5432/samus?sslmode=disable
RETHINKDB_CONTAINER=rethinkdb

postgres-start:
	docker start $(POSTGRES_CONTAINER) 2>/dev/null || docker run -d --name $(POSTGRES_CONTAINER) -p 5432:5432 -e POSTGRES_PASSWORD=postgres postgres:16-alpine

postgres-stop:
	docker stop $(POSTGRES_CONTAINER)

start-api-service:
	$(MAKE) postgres-start
	gin --all run samus.go

install-gin-autoreload:
	go install github.com/codegangsta/gin@latest

run-unit-test:
	testcafe "chrome:headless" tests/navigation.js

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
	@if [ -z ${ID} ];then kill -9 $(ID); else echo "samus.id not found"; fi
	$(eval DLV:=$(shell cat /tmp/dlv.id 2>&1 /dev/null ))
	@if [ -z ${DLV} ];then kill -9 $(DLV); else echo "dlv.Id not found"; fi

# Copy data from a running RethinkDB into Postgres (idempotent; see README).
migrate-from-rethinkdb:
	go run ./cmd/migrate-rb2pg \
		-rb-address localhost:28015 \
		-rb-database samus \
		-pg-dsn "$(POSTGRES_DSN)"
