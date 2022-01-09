NAME ?= rpc-article
REGISTRY_ENDPOINT ?= registry.cn-shanghai.aliyuncs.com
REGISTRY_NAMESPACE ?= hatlonely
IMAGE_TAG ?= $(shell git describe --tags | awk '{print(substr($$0,2,length($$0)))}')

binary=${NAME}
namespace=${REGISTRY_NAMESPACE}
registry=${REGISTRY_ENDPOINT}
repository=${NAME}
version=${IMAGE_TAG}
export GOPROXY=https://goproxy.cn

define BUILD_VERSION
  version: $(shell git describe --tags)
gitremote: $(shell git remote -v | grep fetch | awk '{print $$2}')
   commit: $(shell git rev-parse HEAD)
 datetime: $(shell date '+%Y-%m-%d %H:%M:%S')
 hostname: $(shell hostname):$(shell pwd)
goversion: $(shell go version)
endef
export BUILD_VERSION

build/bin/tunnel-server: cmd/server/main.go $(wildcard internal/*/*.go) Makefile vendor
	mkdir -p build/bin && mkdir -p build/config
	go build -ldflags "-X 'main.Version=$$BUILD_VERSION'" -o build/bin/tunnel-server cmd/server/main.go

build/bin/tunnel-agent: cmd/server/main.go $(wildcard internal/*/*.go) Makefile vendor
	mkdir -p build/bin && mkdir -p build/config
	go build -ldflags "-X 'main.Version=$$BUILD_VERSION'" -o build/bin/tunnel-agent cmd/agent/main.go

build: build/bin/tunnel-server build/bin/tunnel-agent

clean:
	rm -rf build

test:
	@go test -gcflags=all=-l -cover ./internal/...

vendor: go.mod go.sum
	go mod tidy
	go mod vendor

.PHONY: submodule
submodule:
	git submodule init
	git submodule update

.PHONY: image
image:
	docker build --tag=${namespace}/${repository}:${version} .
