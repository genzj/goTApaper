#!/bin/bash

pushd "$(dirname "$0")" >/dev/null 2>&1
depth=2
echo "Working dir: $(pwd)"
echo "Depth: ${depth}"
go run ./vendor/github.com/smartystreets/goconvey/goconvey.go -depth "${depth}"
