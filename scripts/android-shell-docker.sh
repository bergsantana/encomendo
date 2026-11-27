#!/usr/bin/env bash
# Runs ./gradlew for the Android shell (web/apps/android/android) inside the
# same SDK container and volumes as scripts/mobile-docker.sh, for machines
# without an Android SDK (and for Claude sessions). Run `cap sync` on the host
# first; the pnpm scripts do both:
#
#   pnpm --filter @encomendo/android build:debug
#   pnpm --filter @encomendo/android build:release
#   scripts/android-shell-docker.sh testDebugUnitTest
#
# Release signing: export ENCOMENDO_KEYSTORE (a path on this machine),
# ENCOMENDO_KEYSTORE_PASSWORD, ENCOMENDO_KEY_ALIAS and ENCOMENDO_KEY_PASSWORD.
# The keystore is mounted read-only; the passwords pass by name, so they never
# appear in the docker command line. A keystore.properties next to
# settings.gradle also works when its storeFile is inside the repository.
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

signing=()
if [[ -n "${ENCOMENDO_KEYSTORE:-}" ]]; then
  keystore="$(realpath "$ENCOMENDO_KEYSTORE")"
  if [[ ! -f "$keystore" ]]; then
    echo "ENCOMENDO_KEYSTORE does not name a file: $ENCOMENDO_KEYSTORE" >&2
    exit 1
  fi
  signing+=(-v "$keystore":/run/encomendo/release.keystore:ro -e ENCOMENDO_KEYSTORE=/run/encomendo/release.keystore)
fi
for name in ENCOMENDO_KEYSTORE_PASSWORD ENCOMENDO_KEY_ALIAS ENCOMENDO_KEY_PASSWORD VERSION_CODE VERSION_NAME; do
  if [[ -n "${!name:-}" ]]; then
    signing+=(-e "$name")
  fi
done

# ANDROID_USER_HOME keeps the SDK's debug key in the Gradle volume: a fresh key
# per container run would make every debug APK refuse to install over the last.
exec docker run --rm -u "$(id -u):$(id -g)" \
  -e HOME=/tmp/home -e GRADLE_USER_HOME=/gradle-home -e ANDROID_USER_HOME=/gradle-home/android-user \
  "${signing[@]}" \
  -v "$root":/work \
  -v encomendo-gradle-home:/gradle-home \
  -v encomendo-android-sdk:/opt/android-sdk-linux \
  -w /work/web/apps/android/android "$image" \
  ./gradlew "$@" --console=plain
