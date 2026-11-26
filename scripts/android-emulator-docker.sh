#!/usr/bin/env bash
# Runs an Android emulator in Docker, for checking the app against the local
# stack where no Android Studio is installed (milestone 2, step 2.11).
#
#   scripts/android-emulator-docker.sh start     # boot it (minutes; see below)
#   scripts/android-emulator-docker.sh install   # install the debug APK
#   scripts/android-emulator-docker.sh adb <args> # run adb against it
#   scripts/android-emulator-docker.sh stop
#
# Build the APK first, pointed at the stack's API port (API_PORT in .env):
#   scripts/mobile-docker.sh :androidApp:assembleDebug -Pencomendo.apiBaseUrl=http://10.0.2.2:${API_PORT:-8081}
#
# The container uses the host network, so 10.0.2.2 inside the emulator is
# the host's loopback, where the stack listens. The SDK, the emulator and one
# x86_64 system image live in their own volume, apart from the build SDK.
#
# With /dev/kvm (nested virtualization on in WSL, or a Linux host) the
# emulator boots in about a minute. Without it, the script forces software
# emulation (ANDROID_I_WANT_MY_TCG): it boots in about 7 minutes and every
# screen is slow, and a "System UI isn't responding" dialog may need "Wait".
# The AVD has a hardware keyboard, so the on-screen keyboard (and the
# keyboard inset) cannot be checked with it.
set -euo pipefail

image="ghcr.io/cirruslabs/android-sdk:36"
volume="encomendo-android-emulator"
name="encomendo-emulator"
port=5560
serial="emulator-$port"
sysimg="system-images;android-30;default;x86_64"
root="$(git rev-parse --show-toplevel)"
apk="$root/mobile/androidApp/build/outputs/apk/debug/androidApp-debug.apk"

sdk() { docker run --rm -u 0 -e ANDROID_HOME=/sdk -e ANDROID_SDK_ROOT=/sdk -v "$volume":/sdk "$image" "$@"; }

prepare() {
  docker volume inspect "$volume" >/dev/null 2>&1 || docker volume create "$volume" >/dev/null
  sdk sh -c "test -x /sdk/emulator/emulator || yes | sdkmanager --sdk_root=/sdk --install emulator platform-tools '$sysimg' >/dev/null"
  # avdmanager in the image ignores the volume's SDK, so the AVD is written by hand.
  sdk sh -c 'mkdir -p /sdk/avd/e2e.avd
    printf "avd.ini.encoding=UTF-8\npath=/sdk/avd/e2e.avd\ntarget=android-30\n" > /sdk/avd/e2e.ini
    printf "image.sysdir.1=system-images/android-30/default/x86_64/\nabi.type=x86_64\nhw.cpu.arch=x86_64\nhw.ramSize=2048\nhw.lcd.width=720\nhw.lcd.height=1280\nhw.lcd.density=320\ndisk.dataPartition.size=2G\nhw.keyboard=yes\n" > /sdk/avd/e2e.avd/config.ini'
}

start() {
  prepare
  local accel=(-accel auto) kvm=() tcg=()
  if [ -e /dev/kvm ]; then kvm=(--device /dev/kvm); else accel=(-accel off); tcg=(-e ANDROID_I_WANT_MY_TCG=yes); fi
  docker rm -f "$name" >/dev/null 2>&1 || true
  docker run -d --name "$name" --network host -u 0 "${kvm[@]}" "${tcg[@]}" \
    -e ANDROID_HOME=/sdk -e ANDROID_SDK_ROOT=/sdk -e ANDROID_AVD_HOME=/sdk/avd -v "$volume":/sdk "$image" \
    /sdk/emulator/emulator -avd e2e -no-window -no-audio -no-boot-anim -no-snapshot "${accel[@]}" \
    -gpu swiftshader_indirect -port "$port" >/dev/null
  echo "booting $serial; waiting for it (up to 30 minutes without KVM)..."
  local deadline=$((SECONDS + 1800))
  until [ "$(docker exec "$name" /sdk/platform-tools/adb -s "$serial" shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ]; do
    [ "$SECONDS" -lt "$deadline" ] || { echo "not booted after 30 minutes" >&2; exit 1; }
    sleep 10
  done
  echo "booted after ${SECONDS}s"
}

case "${1:-}" in
  start) start ;;
  install)
    test -f "$apk" || { echo "build the APK first (see the header)" >&2; exit 1; }
    docker cp "$apk" "$name":/tmp/app.apk
    docker exec "$name" /sdk/platform-tools/adb -s "$serial" install -r /tmp/app.apk ;;
  adb) shift; docker exec "$name" /sdk/platform-tools/adb -s "$serial" "$@" ;;
  stop) docker rm -f "$name" >/dev/null && echo stopped ;;
  *) sed -n '2,10p' "$0"; exit 1 ;;
esac
