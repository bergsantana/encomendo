import { describe, expect, it } from 'vitest'

import config, { DEBUG_ADMIN_URL, ERROR_PAGE, RELEASE_ADMIN_URL, shellBuild, shellConfig } from './capacitor.config'

describe('the Capacitor config', () => {
  it('opens the hosted admin in release, with the app id the frozen native app used', () => {
    const release = shellConfig('release')

    expect(release.appId).toBe('br.com.encomendo.app')
    expect(release.appName).toBe('Encomendo')
    expect(RELEASE_ADMIN_URL).toBe('https://admin.encomendo.com.br')
    expect(release.server?.url).toBe('https://admin.encomendo.com.br')
  })

  it('opens the local stack admin host in debug', () => {
    expect(DEBUG_ADMIN_URL).toBe('http://admin.localhost:8080')
    expect(shellConfig('debug').server?.url).toBe('http://admin.localhost:8080')
  })

  it.each(['debug', 'release'] as const)('lets no other host navigate inside the %s app', (build) => {
    expect(shellConfig(build).server?.allowNavigation).toEqual([])
  })

  it.each(['debug', 'release'] as const)('refuses mixed content and never sets cleartext in %s', (build) => {
    const built = shellConfig(build)

    expect(built.android?.allowMixedContent).toBe(false)
    // server.cleartext would add usesCleartextTraffic to the merged manifest.
    expect(built.server?.cleartext).toBeUndefined()
  })

  it('turns WebView debugging on only in debug', () => {
    expect(shellConfig('release').android?.webContentsDebuggingEnabled).toBe(false)
    expect(shellConfig('debug').android?.webContentsDebuggingEnabled).toBe(true)
  })

  it('keeps native HTTP and cookies off, so the admin uses the WebView as a browser does', () => {
    const plugins = shellConfig('release').plugins

    expect(plugins?.CapacitorHttp).toEqual({ enabled: false })
    expect(plugins?.CapacitorCookies).toEqual({ enabled: false })
  })

  it('shows the bundled offline page when the admin cannot load', () => {
    expect(shellConfig('release').server?.errorPath).toBe(ERROR_PAGE)
    expect(shellConfig('release').webDir).toBe('www')
  })

  it('builds release unless debug is asked for by name', () => {
    expect(shellBuild({})).toBe('release')
    expect(shellBuild({ ENCOMENDO_SHELL_BUILD: '' })).toBe('release')
    expect(shellBuild({ ENCOMENDO_SHELL_BUILD: 'release' })).toBe('release')
    expect(shellBuild({ ENCOMENDO_SHELL_BUILD: 'debug' })).toBe('debug')
    expect(() => shellBuild({ ENCOMENDO_SHELL_BUILD: 'Debug' })).toThrow('ENCOMENDO_SHELL_BUILD')
  })

  it('exports the config of the build named in the environment', () => {
    const expected = shellBuild(process.env)

    expect(config).toEqual(shellConfig(expected))
  })
})
