GO111MODULE=off
GOARCH=amd64
GOOS=darwin
# GOOS=linux
GO_PACKAGE=github.com/alibaba/open-simulator
CGO_ENABLED=0

COMMITID=$(shell git rev-parse --short HEAD)
VERSION=dev
LD_FLAGS=-ldflags "-X '${GO_PACKAGE}/cmd/version.VERSION=$(VERSION)' -X '${GO_PACKAGE}/cmd/version.COMMITID=$(COMMITID)'"

OUTPUT_DIR=./bin
BINARY_NAME=simon

all: build

.PHONY: build 
build:
	GO111MODULE=$(GO111MODULE) GOARCH=$(GOARCH) GOOS=$(GOOS) CGO_ENABLED=0 go build -trimpath $(LD_FLAGS) -v -o $(OUTPUT_DIR)/$(BINARY_NAME) ./cmd
	# chmod +x $(OUTPUT_DIR)/$(BINARY_NAME)
	# bin/simon apply -i -f ./example/simon-config.yaml

.PHONY: test 
test:
	go test -v ./...

.PHONY: clean 
clean:
	rm -rf ./bin || true

# release builds a GitHub release using goreleaser within the build container.
#
# To dry-run the release, which will build the binaries/artifacts locally but
# will *not* create a GitHub release:
#		GITHUB_TOKEN=an-invalid-token-so-you-dont-accidentally-push-release \
#		RELEASE_NOTES_FILE=changelogs/CHANGELOG-1.2.md \
#		PUBLISH=false \
#		make release
#
# To run the release, which will publish a *DRAFT* GitHub release in github.com/vmware-tanzu/velero
# (you still need to review/publish the GitHub release manually):
#		GITHUB_TOKEN=your-github-token \
#		RELEASE_NOTES_FILE=changelogs/CHANGELOG-1.2.md \
#		PUBLISH=true \
#		make release
.PHONY: release 
release:
	GITHUB_TOKEN=$(GITHUB_TOKEN)
	RELEASE_NOTES_FILE=$(RELEASE_NOTES_FILE)
	PUBLISH=$(PUBLISH)
	./hack/goreleaser.sh
