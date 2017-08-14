.PHONY: build build-alpine clean test help default example

UNAME := $(shell uname)

ifeq ($(UNAME), Linux)
	BIN_NAME := goTApaper
	GO_DAEMON_LDFLAGS :=
else
	BIN_NAME := goTApaper.exe goTApaper-console.exe
	GO_DAEMON_LDFLAGS := -H windowsgui
endif

VERSION := $(shell grep "const Version " cmd/version.go | sed -E 's/.*"(.+)"$$/\1/')
RELEASE ?= DEV
GIT_COMMIT=$(shell git rev-parse HEAD)
GIT_DIRTY=$(shell test -n "`git status --porcelain`" && echo "+CHANGES" || true)
IMAGE_NAME := "genzj/goTApaper"

TARGET_DIR := bin

BIN_TARGET := $(addprefix $(TARGET_DIR)/,$(BIN_NAME))

# NOTE: I exclude the vendor source folder because it's TOO HUGE!
# So after modify vendor source (rarely happens) or glide-update (may happen),
# make clean before make.
GO_SOURCES := $(shell find -type f -name '*.go' -not -iwholename '*/vendor/*')

EXAMPLE_SOURCES := $(wildcard ./*.example)
EXAMPLE_TARGETS := $(addprefix $(TARGET_DIR)/,$(EXAMPLE_SOURCES))

I18N_TARGET_DIR := $(TARGET_DIR)/i18n
I18N_SOURCES := $(wildcard ./*.all.json)
I18N_TARGET := $(addprefix $(I18N_TARGET_DIR)/,$(I18N_SOURCES))

STRING_DEFINES := github.com/genzj/goTApaper/cmd.GitCommit=${GIT_COMMIT}${GIT_DIRTY}
STRING_DEFINES += github.com/genzj/goTApaper/cmd.VersionPrerelease=$(RELEASE)

GO_LDFLAGS := -s -w
GO_LDFLAGS += $(addprefix -X ,$(STRING_DEFINES))

default: build

help:
	@echo 'Management commands for goTApaper:'
	@echo
	@echo 'Usage:'
	@echo '    make build           Compile the project.'
	@echo '    make build-alpine    Compile optimized for alpine linux.'
	@echo '    make build-docker    Build inside an alpine docker container'
	@echo '    make examples        Copy example files to target directory'
	@echo '    make i18n            Copy translation files to i18n subdirectory under the target directory'
	@echo '    make package         Build final docker image with just the go binary inside'
	@echo '    make tag             Tag image created by package with latest, git commit and version'
	@echo '    make test            Run tests on a compiled project.'
	@echo '    make push            Push tagged images to registry'
	@echo '    make clean           Clean the directory tree.'
	@echo

build: $(TARGET_DIR) example i18n $(BIN_TARGET)

$(TARGET_DIR):
	@test -e $(TARGET_DIR) || mkdir -p $(TARGET_DIR)

i18n: $(I18N_TARGET)

$(I18N_TARGET_DIR)/%.all.json: %.all.json
	mkdir -p $(I18N_TARGET_DIR) && \
		cp -f $^ $@

example: $(TARGET_DIR) $(EXAMPLE_TARGETS)


$(TARGET_DIR)/%.example: %.example
	cp $^ $@

$(TARGET_DIR)/%-console.exe: $(GO_SOURCES)
	@echo "building $@ v$(VERSION) $(GIT_COMMIT)$(GIT_DIRTY) win32 console edition"
	@echo "GOPATH=$(GOPATH)"
	go build -ldflags "$(GO_LDFLAGS)" -o $@

$(TARGET_DIR)/%: $(GO_SOURCES)
	@echo "building $@ v$(VERSION) $(GIT_COMMIT)$(GIT_DIRTY)"
	@echo "GOPATH=$(GOPATH)"
	go build -ldflags "$(GO_LDFLAGS) $(GO_DAEMON_LDFLAGS)" -o $@

build-alpine:
	@echo "building ${BIN_NAME} ${VERSION}"
	@echo "GOPATH=${GOPATH}"
	go build -ldflags '-w -linkmode external -extldflags "-static" -X github.com/genzj/goTApaper/cmd.GitCommit=${GIT_COMMIT}${GIT_DIRTY} -X github.com/genzj/goTApaper/cmd.VersionPrerelease=VersionPrerelease=RC' -o bin/${BIN_NAME}

build-docker:
	@echo "building ${BIN_NAME} ${VERSION}"
	docker build -t goTApaper:build -f Dockerfile.build .
	docker run --name=goTApaper -v $(GOPATH):/gopath/  goTApaper:build
	docker rm -f goTApaper

package:
	@echo "building image ${BIN_NAME} ${VERSION} $(GIT_COMMIT)"
	docker build --build-arg VERSION=${VERSION} --build-arg GIT_COMMIT=$(GIT_COMMIT) -t $(IMAGE_NAME):local .

tag:
	@echo "Tagging: latest ${VERSION} $(GIT_COMMIT)"
	docker tag $(IMAGE_NAME):local $(IMAGE_NAME):$(GIT_COMMIT)
	docker tag $(IMAGE_NAME):local $(IMAGE_NAME):${VERSION}
	docker tag $(IMAGE_NAME):local $(IMAGE_NAME):latest

push: tag
	@echo "Pushing docker image to registry: latest ${VERSION} $(GIT_COMMIT)"
	docker push $(IMAGE_NAME):$(GIT_COMMIT)
	docker push $(IMAGE_NAME):${VERSION}
	docker push $(IMAGE_NAME):latest

clean:
	@test ! -e $(TARGET_DIR) || rm -rf $(TARGET_DIR)

test:
	go test ./...

compile-messages:
	goi18n *.all.json *.untranslated.json

extract-messages:
	goi18n *.all.json
