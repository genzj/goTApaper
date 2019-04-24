.PHONY: build build-all build-windows-console clean test help default example

UNAME := $(shell uname)

ifeq ($(UNAME), Linux)
	BIN_NAME := goTApaper
	GO_DAEMON_LDFLAGS :=
else
	BIN_NAME := goTApaper.exe goTApaper-console.exe
	GO_DAEMON_LDFLAGS := -H windowsgui
endif

VERSION := $(shell git describe --match 'REV_*' --debug | sed -e's/-.*//' -e 's/REV_//' -e's/_/./g')
RELEASE ?= DEV
GIT_COMMIT=$(shell git rev-parse HEAD)
GIT_DIRTY=$(shell test -n "`git status --porcelain`" && echo "+CHANGES" || true)
IMAGE_NAME := "genzj/goTApaper"

TARGET_DIR := bin

# NOTE: I exclude the vendor source folder because it's TOO HUGE!
# So after modify vendor source (rarely happens) or glide-update (may happen),
# make clean before make.
GO_SOURCES := $(shell find -type f -name '*.go' -not -iwholename '*/vendor/*')

EXAMPLE_SOURCES := $(wildcard ./*.example)
EXAMPLE_TARGETS := $(addprefix $(TARGET_DIR)/,$(EXAMPLE_SOURCES))

I18N_TARGET_DIR := $(TARGET_DIR)/i18n
I18N_SOURCES := $(wildcard ./*.all.json)
I18N_TARGET := $(addprefix $(I18N_TARGET_DIR)/,$(I18N_SOURCES))

STRING_DEFINES += github.com/genzj/goTApaper/cmd.Version=$(VERSION)
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
	@echo '    make examples        Copy example files to target directory'
	@echo '    make i18n            Copy translation files to i18n subdirectory under the target directory'
	@echo '    make test            Run tests on a compiled project.'
	@echo '    make clean           Clean the directory tree.'
	@echo

build: $(TARGET_DIR) build-all build-windows-console example i18n

$(TARGET_DIR):
	@test -e $(TARGET_DIR) || mkdir -p $(TARGET_DIR)

i18n: $(I18N_TARGET)

$(I18N_TARGET_DIR)/%.all.json: %.all.json
	mkdir -p $(I18N_TARGET_DIR) && \
		cp -f $^ $@

example: $(TARGET_DIR) $(EXAMPLE_TARGETS)


$(TARGET_DIR)/%.example: %.example
	cp $^ $@

build-windows-console: $(GO_SOURCES)
	@echo "building $@ v$(VERSION) $(GIT_COMMIT)$(GIT_DIRTY) win32 console edition"
	@echo "GOPATH=$(GOPATH)"
	cd $(TARGET_DIR) && \
	    gox -arch "amd64 386" -os "windows" -ldflags "$(GO_LDFLAGS)" -output "{{.Dir}}-$(VERSION)-{{.OS}}-{{.Arch}}-console" ../...

build-all: $(GO_SOURCES)
	@echo "building $@ v$(VERSION) $(GIT_COMMIT)$(GIT_DIRTY)"
	@echo "GOPATH=$(GOPATH)"
	cd $(TARGET_DIR) && \
	    GOX_WINDOWS_386_LDFLAGS="$(GO_LDFLAGS) -H windowsgui" \
	    GOX_WINDOWS_amd64_LDFLAGS="$(GO_LDFLAGS) -H windowsgui" \
	    gox -arch "amd64 386" -os "windows linux" -ldflags "$(GO_LDFLAGS)" -output "{{.Dir}}-$(VERSION)-{{.OS}}-{{.Arch}}"  ../...

clean:
	@test ! -e $(TARGET_DIR) || rm -rf $(TARGET_DIR)

test:
	go test ./...

compile-messages:
	goi18n *.all.json *.untranslated.json

extract-messages:
	goi18n *.all.json
