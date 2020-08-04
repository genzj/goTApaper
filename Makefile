.PHONY: build build-all build-windows-console clean test help default example generate

UNAME := $(shell uname)

VERSION := $(shell git describe --match 'v[0-9]*' --debug | sed -e's/-.*//' -e 's/v//')
RELEASE ?= DEV
GIT_COMMIT=$(shell git rev-parse HEAD)
GIT_DIRTY=$(shell test -n "`git status --porcelain`" && echo "+CHANGES" || true)
IMAGE_NAME := "genzj/goTApaper"

TARGET_DIR := bin

# NOTE: I exclude the vendor source folder because it's TOO HUGE!
# So after modify vendor source (rarely happens) or glide-update (may happen),
# make clean before make.
GO_SOURCES := $(shell find . -type f -name '*.go' -not -iwholename '*/vendor/*')

EXAMPLE_SOURCES := $(wildcard ./examples/*.example)
EXAMPLE_TARGETS := $(addprefix $(TARGET_DIR)/,$(EXAMPLE_SOURCES:./examples/%=%))

I18N_TARGET_DIR := $(TARGET_DIR)/i18n
I18N_SOURCES := $(wildcard ./*.all.json)
I18N_TARGET := $(addprefix $(I18N_TARGET_DIR)/,$(I18N_SOURCES))

STRING_DEFINES := github.com/genzj/goTApaper/cmd.Version=$(VERSION)
STRING_DEFINES += github.com/genzj/goTApaper/cmd.GitCommit=${GIT_COMMIT}${GIT_DIRTY}
STRING_DEFINES += github.com/genzj/goTApaper/cmd.VersionPrerelease=$(RELEASE)

GO_LDFLAGS := -s -w
GO_LDFLAGS += $(addprefix -X ,$(STRING_DEFINES))

default: build

help:
	@echo 'Management commands for goTApaper:'
	@echo
	@echo 'Usage:'
	@echo '    make build           Compile the project.'
	@echo '    make examples        Copy example files to target directory'
	@echo '    make i18n            Copy translation files to i18n subdirectory under the target directory'
	@echo '    make test            Run tests on a compiled project.'
	@echo '    make clean           Clean the directory tree.'
	@echo

build: $(TARGET_DIR) build-other example i18n

build-windows: $(TARGET_DIR) build-windows-gui build-windows-console example i18n

$(TARGET_DIR):
	@test -e $(TARGET_DIR) || mkdir -p $(TARGET_DIR)

i18n: $(I18N_TARGET)

$(I18N_TARGET_DIR)/%.all.json: %.all.json
	mkdir -p $(I18N_TARGET_DIR) && \
		cp -f $^ $@

example: $(TARGET_DIR) $(EXAMPLE_TARGETS)


$(TARGET_DIR)/%.example: examples/%.example
	cp $^ $@

build-windows-console: generate $(GO_SOURCES)
	@echo "building $@ v$(VERSION) $(GIT_COMMIT)$(GIT_DIRTY) Win console edition"
	@echo "GOPATH=$(GOPATH)"
	cd $(TARGET_DIR) && \
	    gox -cgo -arch "amd64" -os "windows" -ldflags "$(GO_LDFLAGS)" -output "{{.Dir}}-$(VERSION)-{{.OS}}-{{.Arch}}-console" ../...

build-windows-gui: generate $(GO_SOURCES)
	@echo "building $@ v$(VERSION) $(GIT_COMMIT)$(GIT_DIRTY) Win gui edition"
	@echo "GOPATH=$(GOPATH)"
	cd $(TARGET_DIR) && \
	    GOX_WINDOWS_386_LDFLAGS="$(GO_LDFLAGS) -H windowsgui" \
	    GOX_WINDOWS_AMD64_LDFLAGS="$(GO_LDFLAGS) -H windowsgui" \
	    gox -cgo -arch "amd64" -os "windows" -ldflags "$(GO_LDFLAGS)" -output "{{.Dir}}-$(VERSION)-{{.OS}}-{{.Arch}}"  ../...

build-other: generate $(GO_SOURCES)
	@echo "building $@ v$(VERSION) $(GIT_COMMIT)$(GIT_DIRTY)"
	@echo "GOPATH=$(GOPATH)"
	cd $(TARGET_DIR) && \
	    gox -arch "amd64 386" -os "linux" -osarch "darwin/amd64" -ldflags "$(GO_LDFLAGS)" -output "{{.Dir}}-$(VERSION)-{{.OS}}-{{.Arch}}"  ../...

clean:
	-rm -rf $(TARGET_DIR) data/example_vfsdata.go

test:
	go test ./...

compile-messages:
	goi18n *.all.json *.untranslated.json

extract-messages:
	goi18n *.all.json

generate:
	cd generate && { go generate -v || exit 1 ; cd .. ; }
