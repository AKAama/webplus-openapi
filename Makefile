export SHELL:=/bin/bash
export SHELLOPTS:=$(if $(SHELLOPTS),$(SHELLOPTS):)pipefail:errexit

MAKEFLAGS += --no-builtin-rules
.SUFFIXES:
PACKAGE_NAME		  := webplus-openai

BUILD_DATE            := $(shell date +'%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT            := $(shell git rev-parse HEAD)
GIT_REMOTE            := origin
GIT_BRANCH            := $(shell git rev-parse --symbolic-full-name --verify --quiet --abbrev-ref HEAD)
GIT_TAG               := $(shell git describe --tags --abbrev=0  2> /dev/null || echo untagged)
GIT_TREE_STATE        := $(shell if [ -z "`git status --porcelain`" ]; then echo "clean"; else echo "dirty"; fi)
RELEASE_TAG           := $(shell if [[ "$(GIT_TAG)" =~ ^v[0-9]+\.[0-9]+\.[0-9]+.*$$ ]]; then echo "true"; else echo "false"; fi)
DEV_BRANCH            := $(shell [ $(GIT_BRANCH) = master ] || [ `echo $(GIT_BRANCH) | cut -c -8` = release- ] || [ `echo $(GIT_BRANCH) | cut -c -4` = dev- ] || [ $(RELEASE_TAG) = true ] && echo false || echo true)
SRC                   := $(pwd)
GREP_LOGS             := ""
VERSION               := $(shell cat .version)
DOCKER_PUSH           := true

CGO_ENABLED ?= 1
WASM_ENABLED ?= 1

GO := CGO_ENABLED=$(CGO_ENABLED) GO111MODULE=on go
GOVERSION := $(shell cat ./.go-version)
GOARCH := $(shell go env GOARCH)
GOOS := $(shell go env GOOS)
DISABLE_CGO := CGO_ENABLED=0

override LDFLAGS = -X $(PACKAGE_NAME)/internal/util.version=${VERSION} \
  -X $(PACKAGE_NAME)/internal/util.buildDate=${BUILD_DATE} \
  -X $(PACKAGE_NAME)/internal/util.gitCommit=${GIT_COMMIT} \
  -X $(PACKAGE_NAME)/internal/util.gitTreeState=${GIT_TREE_STATE}

ifneq ($(GIT_TAG),)
	override LDFLAGS += -X $(PACKAGE_NAME)/internal/util.gitTag=${GIT_TAG}
endif

ifndef $(GOPATH)
	GOPATH=$(shell go env GOPATH)
	export GOPATH
endif


build:
	$(GO) build $(GO_TAGS) -o out/$(APP_NAME)-$(GOOS)-$(GOARCH) -ldflags '$(LDFLAGS)' $(MAIN_FILE)

.PHONY: build-linux-static
build-linux-static:
	@$(MAKE) GOOS=linux GOARCH=amd64 build  WASM_ENABLED=0 CGO_ENABLED=0

.PHONY: image-push
image-push:
	docker push $(IMAGE):$(VERSION)
	docker rmi $(IMAGE):$(VERSION)

# 默认目标
.PHONY: all
all: build_multi_arch_image

# 构建二进制文件
.PHONY: build_multi_arch_binary
build_multi_arch_binary:
	make -f $(CURRENT_MAKEFILE_NAME) GOOS=linux GOARCH=amd64 build  WASM_ENABLED=0 CGO_ENABLED=0
	make -f $(CURRENT_MAKEFILE_NAME) GOOS=linux GOARCH=arm64 build  WASM_ENABLED=0 CGO_ENABLED=0

# 构建并推送Docker镜像
.PHONY: build_multi_arch_image
build_multi_arch_image:
	make -f $(CURRENT_MAKEFILE_NAME) build_multi_arch_binary
	docker buildx build -f $(DOCKER_FILE) --platform linux/amd64,linux/arm64 \
		--build-arg APP_NAME=$(APP_NAME) \
        -t $(IMAGE):$(VERSION) --push .
	@make -f $(CURRENT_MAKEFILE_NAME) delete_file
delete_file:
	rm -f out/$(APP_NAME)-linux*
