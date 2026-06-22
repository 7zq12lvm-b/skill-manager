#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_BUNDLE="${ROOT_DIR}/build/bin/skill-manager.app"
OUTPUT_DMG="${OUTPUT_DMG:-${ROOT_DIR}/build/bin/skill-manager.dmg}"
VOLUME_NAME="${VOLUME_NAME:-AI Agent Skill Manager}"
PLATFORM="${PLATFORM:-darwin/arm64}"
WAILS_BIN="${WAILS_BIN:-}"
DMG_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/skill-manager-dmg.XXXXXX")"

cleanup() {
  rm -rf "${DMG_ROOT}"
}
trap cleanup EXIT

if [[ -z "${WAILS_BIN}" ]]; then
  if [[ -x "/Users/yusuf/go/bin/wails" ]]; then
    WAILS_BIN="/Users/yusuf/go/bin/wails"
  elif WAILS_BIN="$(command -v wails 2>/dev/null)"; then
    :
  else
    echo "Wails CLI not found. Set WAILS_BIN=/path/to/wails or install wails." >&2
    exit 1
  fi
fi

if [[ "$(uname -s)" != "Darwin" ]]; then
  echo "DMG packaging requires macOS." >&2
  exit 1
fi

if ! command -v hdiutil >/dev/null 2>&1; then
  echo "hdiutil is required to create the DMG." >&2
  exit 1
fi

echo "Building Wails app for ${PLATFORM}..."
"${WAILS_BIN}" build -clean -platform "${PLATFORM}"

if [[ ! -d "${APP_BUNDLE}" ]]; then
  echo "Expected app bundle not found: ${APP_BUNDLE}" >&2
  exit 1
fi

echo "Preparing DMG root..."
cp -R "${APP_BUNDLE}" "${DMG_ROOT}/"
ln -s /Applications "${DMG_ROOT}/Applications"

mkdir -p "$(dirname "${OUTPUT_DMG}")"

echo "Creating DMG: ${OUTPUT_DMG}"
hdiutil create \
  -volname "${VOLUME_NAME}" \
  -srcfolder "${DMG_ROOT}" \
  -ov \
  -format UDZO \
  "${OUTPUT_DMG}"

echo "Done: ${OUTPUT_DMG}"
