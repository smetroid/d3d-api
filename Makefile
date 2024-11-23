SHELL := /bin/bash
NAME=rethinkdb

start-api-service:
	docker start rethinkdb
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


createAndStartContainer:
	docker run --name rethinkdb -p 8080:8080 -p 28015:28015 -v "${PWD}:/data" -d rethinkdb

removeContainer:
	docker rm ${NAME}

startContainer:
	docker start ${NAME}

stopContainer:
	docker stop ${NAME}

