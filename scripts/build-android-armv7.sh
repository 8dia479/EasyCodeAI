#!/usr/bin/env bash
set -euo pipefail

ndk_root="${ANDROID_NDK_HOME:-${ANDROID_NDK_ROOT:-}}"
if [ -z "$ndk_root" ] && [ -n "${ANDROID_HOME:-}" ]; then
  ndk_root="$ANDROID_HOME/ndk/27.2.12479018"
fi
if [ -z "$ndk_root" ] || [ ! -d "$ndk_root/toolchains/llvm/prebuilt/linux-x86_64/bin" ]; then
  echo "Android NDK r27c is required. Set ANDROID_NDK_HOME or ANDROID_NDK_ROOT." >&2
  exit 1
fi

cc="$ndk_root/toolchains/llvm/prebuilt/linux-x86_64/bin/armv7a-linux-androideabi31-clang"
if [ ! -x "$cc" ]; then
  echo "Android ARMv7 compiler not found: $cc" >&2
  exit 1
fi

mkdir -p dist
GOOS=android GOARCH=arm GOARM=7 CGO_ENABLED=1 CC="$cc" \
  go build -trimpath -ldflags="-s -w" -o dist/easycode-android-armv7 ./cmd/easycode

echo "Built dist/easycode-android-armv7"
