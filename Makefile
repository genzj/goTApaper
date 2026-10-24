.PHONY: build build-all build-windows-console clean test help default example generate frontend frontend-deps

UNAME := $(shell uname)

VERSION := $(shell git describe --tags --match 'v[0-9]*' | sed -e's/-.*//' -e 's/v//')
RELEASE ?= DEV
GIT_COMMIT=$(shell git rev-parse HEAD)
GIT_DIRTY=$(shell test -n "`git status --porcelain`" && echo "+CHANGES" || true)
IMAGE_NAME := "genzj/goTApaper"

TARGET_DIR := bin
DARWIN_APP_NAME := goTApaper.app

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
	@echo '    make frontend        Build the web interface into frontend/dist'
	@echo '    make examples        Copy example files to target directory'
	@echo '    make i18n            Copy translation files to i18n subdirectory under the target directory'
	@echo '    make test            Run tests on a compiled project.'
	@echo '    make clean           Clean the directory tree.'
	@echo

build: build-os-linux-amd64 build-os-linux-i386

build-os-%: $(TARGET_DIR) generate frontend $(GO_SOURCES)
	make go-$@ example i18n

FRONTEND_DIR := frontend
FRONTEND_DIST := $(FRONTEND_DIR)/dist/index.html
FRONTEND_SOURCES := $(shell find $(FRONTEND_DIR)/src -type f 2>/dev/null) \
	$(FRONTEND_DIR)/package.json $(FRONTEND_DIR)/index.html \
	$(FRONTEND_DIR)/vite.config.ts

# The built interface, embedded into the binary by frontend/assets.go. It is a
# build prerequisite rather than a separate step because //go:embed needs the
# directory to hold the real bundle, not just the committed placeholder.
frontend: $(FRONTEND_DIST)

$(FRONTEND_DIST): $(FRONTEND_SOURCES)
	cd $(FRONTEND_DIR) && { test -d node_modules || npm install ; } && npm run build

# npm ci for a reproducible install, used by CI where the lockfile is
# authoritative.
frontend-deps:
	cd $(FRONTEND_DIR) && npm ci

$(TARGET_DIR):
	@test -e $(TARGET_DIR) || mkdir -p $(TARGET_DIR)

i18n: $(I18N_TARGET)

$(I18N_TARGET_DIR)/%.all.json: %.all.json
	mkdir -p $(I18N_TARGET_DIR) && \
		cp -f $^ $@

example: $(TARGET_DIR) $(EXAMPLE_TARGETS)


$(TARGET_DIR)/%.example: examples/%.example
	cp $^ $@

go-build-os-windows: go-build-os-windows-gui go-build-os-windows-console

go-build-os-windows-console:
	@echo "building $@ v$(VERSION) $(GIT_COMMIT)$(GIT_DIRTY) Win console edition"
	@echo "GOPATH=$(GOPATH)"
	cd $(TARGET_DIR) && \
	    gox -cgo -arch "amd64" -os "windows" -ldflags "$(GO_LDFLAGS)" -output "{{.Dir}}-$(VERSION)-{{.OS}}-{{.Arch}}-console" ../...

go-build-os-windows-gui:
	@echo "building $@ v$(VERSION) $(GIT_COMMIT)$(GIT_DIRTY) Win gui edition"
	@echo "GOPATH=$(GOPATH)"
	cd $(TARGET_DIR) && \
	    GOX_WINDOWS_386_LDFLAGS="$(GO_LDFLAGS) -H windowsgui" \
	    GOX_WINDOWS_AMD64_LDFLAGS="$(GO_LDFLAGS) -H windowsgui" \
	    gox -cgo -arch "amd64" -os "windows" -ldflags "$(GO_LDFLAGS)" -output "{{.Dir}}-$(VERSION)-{{.OS}}-{{.Arch}}"  ../...

go-build-os-linux-amd64:
	@echo "building $@ v$(VERSION) $(GIT_COMMIT)$(GIT_DIRTY)"
	@echo "GOPATH=$(GOPATH)"
	cd $(TARGET_DIR) && \
	    gox -cgo -arch "amd64" -os "linux"  -ldflags "$(GO_LDFLAGS)" -output "{{.Dir}}-$(VERSION)-{{.OS}}-{{.Arch}}"  ../...

go-build-os-linux-i386:
	@echo "building $@ v$(VERSION) $(GIT_COMMIT)$(GIT_DIRTY)"
	@echo "GOPATH=$(GOPATH)"
	cd $(TARGET_DIR) && \
	    PKG_CONFIG_PATH="/usr/lib/i386-linux-gnu/pkgconfig:/usr/lib32/pkgconfig" \
	    gox -cgo -arch "386" -os "linux"  -ldflags "$(GO_LDFLAGS)" -output "{{.Dir}}-$(VERSION)-{{.OS}}-{{.Arch}}"  ../...

go-build-os-darwin-amd64:
	@echo "building $@ v$(VERSION) $(GIT_COMMIT)$(GIT_DIRTY)"
	@echo "GOPATH=$(GOPATH)"
	cd $(TARGET_DIR) && \
	    gox -cgo -osarch "darwin/amd64" -ldflags "$(GO_LDFLAGS)" -output "{{.Dir}}-$(VERSION)-{{.OS}}-{{.Arch}}"  ../... && \
	    ../generate-dmg.sh "$(VERSION)" "amd64"

go-build-os-darwin-arm64:
	@echo "building $@ v$(VERSION) $(GIT_COMMIT)$(GIT_DIRTY)"
	@echo "GOPATH=$(GOPATH)"
	cd $(TARGET_DIR) && \
	    gox -cgo -osarch "darwin/arm64" -ldflags "$(GO_LDFLAGS)" -output "{{.Dir}}-$(VERSION)-{{.OS}}-{{.Arch}}"  ../... && \
	    ../generate-dmg.sh "$(VERSION)" "arm64"

clean:
	-rm -rf $(TARGET_DIR) data/example_vfsdata.go
	# The placeholder stays: frontend/assets.go embeds this directory, and
	# //go:embed fails on a missing one.
	-find $(FRONTEND_DIR)/dist -mindepth 1 ! -name .gitkeep -delete

test:
	go test ./...

compile-messages:
	goi18n *.all.json *.untranslated.json

extract-messages:
	goi18n *.all.json

generate:
	cd generate && { go generate -v || exit 1 ; cd .. ; }
