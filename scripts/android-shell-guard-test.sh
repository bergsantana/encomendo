#!/usr/bin/env bash
# Proves the Android shell's release guards in web/apps/android/android/app/build.gradle refuse what
# they must. Each case runs one Gradle guard task in the SDK container and expects it to fail with
# its message. Needs Node and pnpm on the host (for cap sync) and Docker.
#
#   pnpm --filter @encomendo/android test:guards
#
# It leaves the project synced for release.
set -euo pipefail

root="$(git rev-parse --show-toplevel)"
app="$root/web/apps/android"
gradle="$root/scripts/android-shell-docker.sh"
synced="$app/android/app/src/main/assets/capacitor.config.json"
failures=0

# expect_failure NAME MESSAGE COMMAND...: the command must fail and print MESSAGE.
expect_failure() {
  local name="$1" message="$2"
  shift 2
  local output
  if output="$("$@" 2>&1)"; then
    echo "FAIL $name: the guard let it through"
    failures=$((failures + 1))
  elif grep -qF -- "$message" <<<"$output"; then
    echo "ok   $name"
  else
    echo "FAIL $name: failed without \"$message\""
    tail -20 <<<"$output"
    failures=$((failures + 1))
  fi
}

# tamper JS: rewrite the synced release config with one change.
tamper() {
  node -e '
    const fs = require("fs");
    const [file, change] = process.argv.slice(1);
    const config = JSON.parse(fs.readFileSync(file, "utf8"));
    new Function("config", change)(config);
    fs.writeFileSync(file, JSON.stringify(config, null, "\t"));
  ' "$synced" "$1"
}

sync() {
  (cd "$app" && ENCOMENDO_SHELL_BUILD="$1" pnpm exec cap sync android >/dev/null)
}

without_signing=(env -u ENCOMENDO_KEYSTORE -u ENCOMENDO_KEYSTORE_PASSWORD -u ENCOMENDO_KEY_ALIAS -u ENCOMENDO_KEY_PASSWORD)

sync debug
expect_failure "a debug sync is refused for release" \
  "must open https://admin.encomendo.com.br, not http://admin.localhost:8080" \
  "$gradle" :app:checkReleaseShellConfig

for change in \
  'config.server.url = "https://admin.encomendo.com.br.evil.example"|must open https://admin.encomendo.com.br' \
  'config.server.allowNavigation = ["*.encomendo.com.br"]|must let no other host navigate' \
  'config.android.webContentsDebuggingEnabled = true|must turn WebView debugging off' \
  'config.android.allowMixedContent = true|must refuse mixed content and cleartext' \
  'config.server.cleartext = true|must refuse mixed content and cleartext' \
  'config.android.useLegacyBridge = false|must not set android.useLegacyBridge' \
  'config.android.minWebViewVersion = 60|must require WebView 120'; do
  sync release
  tamper "${change%%|*}"
  expect_failure "release refuses: ${change%%|*}" "${change#*|}" "$gradle" :app:checkReleaseShellConfig
done

sync release
if "$gradle" :app:checkReleaseShellConfig >/dev/null 2>&1; then
  echo "ok   a release sync passes"
else
  echo "FAIL a release sync is refused"
  failures=$((failures + 1))
fi

expect_failure "a release build needs its signing values" \
  "Release signing is not configured. Missing: ENCOMENDO_KEYSTORE" \
  "${without_signing[@]}" "$gradle" :app:checkReleaseSigning

expect_failure "the versionCode never goes below the native app's" \
  "VERSION_CODE must be 100 or more" \
  env VERSION_CODE=5 "$gradle" help

if [[ "$failures" -gt 0 ]]; then
  echo "$failures guard check(s) failed"
  exit 1
fi
echo "every release guard refuses what it must"
