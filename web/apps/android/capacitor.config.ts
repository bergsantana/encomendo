import type { CapacitorConfig } from '@capacitor/cli'

/**
 * ShellBuild names the two Android builds. Each opens its own admin URL, so
 * `cap sync` must run for the build Gradle is about to assemble; the
 * `build:debug` and `build:release` scripts do both in one step.
 */
export type ShellBuild = 'debug' | 'release'

/** RELEASE_ADMIN_URL is the hosted admin every release build opens (ADR 0029). */
export const RELEASE_ADMIN_URL = 'https://admin.encomendo.com.br'

/**
 * DEBUG_ADMIN_URL is the local stack's admin host. nginx serves the admin only
 * for ADMIN_HOST (deploy/web/nginx.conf.template); the phone reaches the
 * host machine's port 8080 through `adb reverse tcp:8080 tcp:8080`, and the
 * WebView resolves `*.localhost` to loopback.
 */
export const DEBUG_ADMIN_URL = 'http://admin.localhost:8080'

/**
 * ERROR_PAGE is the shell's only page of its own: the offline message, shown
 * when the admin cannot be loaded. Capacitor also requires `webDir` to hold
 * an index.html, so the offline page is that file.
 */
export const ERROR_PAGE = 'index.html'

/**
 * MIN_WEBVIEW_VERSION is the oldest Android System WebView (Chromium major version) the shell runs
 * on. Below it Capacitor shows the error page; MainActivity also refuses any WebView without the
 * features that bind the bridge to the admin origin. app/build.gradle checks the synced value.
 */
export const MIN_WEBVIEW_VERSION = 120

/**
 * shellBuild reads ENCOMENDO_SHELL_BUILD. Unset means release, so a plain
 * `cap sync` can never leave a release build pointing at the local stack.
 * Any other value fails, rather than guessing.
 */
export function shellBuild(env: Record<string, string | undefined>): ShellBuild {
  const value = env.ENCOMENDO_SHELL_BUILD
  if (value === undefined || value === '' || value === 'release') return 'release'
  if (value === 'debug') return 'debug'
  throw new Error(`ENCOMENDO_SHELL_BUILD must be "debug" or "release", not "${value}".`)
}

/**
 * shellConfig is the Capacitor config for one build. It guarantees:
 * - the WebView opens the hosted admin, so the page is same-origin with the
 *   API and the refresh cookie and `/auth` origin checks work as in a
 *   browser (ADR 0010);
 * - no other host is allowed to navigate inside the app (`allowNavigation`
 *   is empty, and MainActivity's NavigationPolicy enforces the same lock);
 * - mixed content is refused, and WebView debugging is on only in debug;
 * - the WebView is recent enough, and the legacy bridge (a JavaScript
 *   interface open to every frame) is never set.
 *
 * Cleartext is not set here on purpose: `server.cleartext` would add
 * `usesCleartextTraffic="true"` to the merged manifest. The debug build's
 * network security config allows plain HTTP for the local stack instead.
 */
export function shellConfig(build: ShellBuild): CapacitorConfig {
  return {
    appId: 'br.com.encomendo.app',
    appName: 'Encomendo',
    webDir: 'www',
    server: {
      url: build === 'release' ? RELEASE_ADMIN_URL : DEBUG_ADMIN_URL,
      errorPath: ERROR_PAGE,
      allowNavigation: [],
    },
    android: {
      allowMixedContent: false,
      webContentsDebuggingEnabled: build === 'debug',
      minWebViewVersion: MIN_WEBVIEW_VERSION,
    },
    plugins: {
      // The admin talks to the API with the WebView's own fetch and cookies,
      // exactly as in a browser. `enabled: false` only stops the JavaScript
      // patching of fetch and document.cookie: Capacitor still registers both
      // native plugins, so MainActivity replaces them (NativeBridgeLockdown).
      CapacitorHttp: { enabled: false },
      CapacitorCookies: { enabled: false },
    },
  }
}

export default shellConfig(shellBuild(process.env))
