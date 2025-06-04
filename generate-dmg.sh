#!/bin/bash
set -ex

cd "$(dirname "$(readlink -f "$0")")"

DARWIN_APP_NAME="goTApaper.app"
BUNDLE_DIR="./bin/bundle"

VERSION="$1"
ARCH="$2"

DMG_FILE="./bin/goTApaper-${VERSION}-${ARCH}.dmg"
test -d "$BUNDLE_DIR" && rm -rf "$BUNDLE_DIR"
test -f "$DMG_FILE" && rm "$DMG_FILE"

mkdir -p "$BUNDLE_DIR" && \
  cp -r -f ./contrib/${DARWIN_APP_NAME} "${BUNDLE_DIR}" && \
  cp -f "./bin/goTApaper-${VERSION}-darwin-${ARCH}" "${BUNDLE_DIR}/${DARWIN_APP_NAME}/Contents/MacOS/goTApaper" && \
  create-dmg \
      --volname "goTApaper" \
      --volicon "./contrib/goTApaper.app/Contents/Resources/goTApaper.icns" \
      --background "./contrib/dmg-installer-background.png" \
      --window-pos 200 120 \
      --window-size 1024 560 \
      --icon-size 110 \
      --icon "${DARWIN_APP_NAME}" 346 270 \
      --hide-extension "${DARWIN_APP_NAME}" \
      --app-drop-link 686 270 \
      --hdiutil-verbose \
      "${DMG_FILE}" \
      "${BUNDLE_DIR}/goTApaper.app"
