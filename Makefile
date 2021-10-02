GO111MODULE=off
GOARCH=amd64
GOOS=darwin
# GOOS=linux
GO_PACKAGE=github.com/alibaba/open-simulator
CGO_ENABLED=0

COMMITID=$(shell git rev-parse --short HEAD)
VERSION=v0.1.0-dev
LD_FLAGS=-ldflags "-X '${GO_PACKAGE}/cmd/version.VERSION=$(VERSION)' -X '${GO_PACKAGE}/cmd/version.COMMITID=$(COMMITID)'"

OUTPUT_DIR=./bin
BINARY_NAME=simon

all: build

.PHONY: build 
build:
	GO111MODULE=$(GO111MODULE) GOARCH=$(GOARCH) GOOS=$(GOOS) CGO_ENABLED=0 go build $(LD_FLAGS) -v -o $(OUTPUT_DIR)/$(BINARY_NAME) ./cmd
	chmod +x $(OUTPUT_DIR)/$(BINARY_NAME)
	# bin/simon debug -f ./example/
	# bin/simon apply --kubeconfig=./kubeconfig -f ./example/simple_example_by_huizhi
	# bin/simon apply --kubeconfig=./kubeconfig -f ./example/complicated_example_by_huizhi
	# bin/simon apply --kubeconfig=./kubeconfig -f ./example/more_pods_by_huizhi

.PHONY: test 
test:
	GO111MODULE=$(GO111MODULE) GOARCH=$(GOARCH) GOOS=linux CGO_ENABLED=0 go build $(LD_FLAGS) -v -o $(OUTPUT_DIR)/$(BINARY_NAME) ./cmd
	chmod +x $(OUTPUT_DIR)/$(BINARY_NAME)
	scp $(OUTPUT_DIR)/$(BINARY_NAME) yoda1:/root/

.PHONY: clean 
clean:
	rm -rf ./bin || true