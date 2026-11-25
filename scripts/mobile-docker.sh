#!/usr/bin/env bash
# Runs ./gradlew for mobile/ inside a container that already has JDK 21 and the
# Android SDK, for machines without an Android SDK (and for Claude sessions).
#
#   scripts/mobile-docker.sh check
#   scripts/mobile-docker.sh :androidApp:assembleDebug
#
# The SDK lives in a named volume so Gradle can download the platform the
# build asks for (compileSdk 37 is newer than the image's 36). The licenses
# come with the image. The Gradle cache lives in another volume.
set -euo pipefail

image="ghcr.io/cirruslabs/android-sdk:36"
root="$(git rev-parse --show-toplevel)"

if ! docker volume inspect encomendo-android-sdk >/dev/null 2>&1; then
  docker volume create encomendo-android-sdk >/dev/null
  # First use: the volume starts as a copy of the image's SDK, owned by root.
  docker run --rm -u 0 -v encomendo-android-sdk:/opt/android-sdk-linux "$image" \
    chown -R "$(id -u):$(id -g)" /opt/android-sdk-linux
fi

if ! docker volume inspect encomendo-gradle-home >/dev/null 2>&1; then
  docker volume create encomendo-gradle-home >/dev/null
  docker run --rm -u 0 -v encomendo-gradle-home:/gradle-home "$image" \
    chown "$(id -u):$(id -g)" /gradle-home
fi

exec docker run --rm -u "$(id -u):$(id -g)" \
  -e HOME=/tmp/home -e GRADLE_USER_HOME=/gradle-home \
  -v "$root":/work \
  -v encomendo-gradle-home:/gradle-home \
  -v encomendo-android-sdk:/opt/android-sdk-linux \
  -w /work/mobile "$image" \
  ./gradlew "$@" --continue --console=plain
