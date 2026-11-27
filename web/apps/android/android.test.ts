import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

import { DEBUG_ADMIN_URL } from './capacitor.config'

/**
 * Checks on the native project's hardening that need no device: they read the
 * committed Android files as text. The WebView's behaviour itself is checked
 * on a phone in step 3.5.5 (README.md, Device checks).
 */

function read(path: string): string {
  const text = readFileSync(fileURLToPath(new URL(path, import.meta.url)), 'utf8')
  // Comments explain the rules in words that would match the checks below.
  return text.replace(/<!--[\s\S]*?-->/g, '')
}

interface DomainConfig {
  cleartext: boolean
  domains: { name: string; includeSubdomains: boolean }[]
}

interface NetworkSecurityConfig {
  baseCleartext: string | undefined
  trustAnchors: string[]
  domainConfigs: DomainConfig[]
}

function networkSecurityConfig(path: string): NetworkSecurityConfig {
  const xml = read(path)
  const base = /<base-config\b([^>]*)>([\s\S]*?)<\/base-config>/.exec(xml)
  const domainConfigs = [...xml.matchAll(/<domain-config\b([^>]*)>([\s\S]*?)<\/domain-config>/g)].map(
    ([, attributes = '', body = '']) => ({
      cleartext: /cleartextTrafficPermitted="true"/.test(attributes),
      domains: [...body.matchAll(/<domain\b([^>]*)>([^<]*)<\/domain>/g)].map(([, domainAttributes = '', name = '']) => ({
        name: name.trim(),
        includeSubdomains: /includeSubdomains="true"/.test(domainAttributes),
      })),
    }),
  )
  return {
    baseCleartext: base ? /cleartextTrafficPermitted="(\w+)"/.exec(base[1] ?? '')?.[1] : undefined,
    trustAnchors: [...(base?.[2] ?? '').matchAll(/<certificates\s+src="([^"]+)"/g)].map(([, src = '']) => src),
    domainConfigs,
  }
}

const release = networkSecurityConfig('./android/app/src/main/res/xml/network_security_config.xml')
const debug = networkSecurityConfig('./android/app/src/debug/res/xml/network_security_config.xml')
const manifest = read('./android/app/src/main/AndroidManifest.xml')

describe('the network security configs', () => {
  it('refuse cleartext everywhere in release, trusting only system authorities', () => {
    expect(release).toEqual({ baseCleartext: 'false', trustAnchors: ['system'], domainConfigs: [] })
  })

  it('allow cleartext in debug only for the local stack hosts, without subdomains', () => {
    expect(debug.baseCleartext).toBe('false')
    expect(debug.trustAnchors).toEqual(['system'])
    expect(debug.domainConfigs).toEqual([
      {
        cleartext: true,
        domains: [
          { name: 'admin.localhost', includeSubdomains: false },
          { name: 'localhost', includeSubdomains: false },
        ],
      },
    ])
  })

  it('let the debug build reach the debug admin URL and the local photo storage', () => {
    const cleartextHosts = debug.domainConfigs.flatMap((config) => config.domains.map((domain) => domain.name))

    expect(cleartextHosts).toContain(new URL(DEBUG_ADMIN_URL).hostname)
    // Silo signs photo URLs for localhost:9000 (deploy/docker-compose.yml).
    expect(cleartextHosts).toContain('localhost')
  })
})

describe('the manifest', () => {
  it('points at the network security config and never turns cleartext on itself', () => {
    expect(manifest).toContain('android:networkSecurityConfig="@xml/network_security_config"')
    expect(manifest).not.toMatch(/usesCleartextTraffic/)
  })

  it('keeps app data, the refresh cookie included, out of backups and device transfers', () => {
    expect(manifest).toContain('android:allowBackup="false"')
    expect(manifest).toContain('android:fullBackupContent="false"')
    expect(manifest).toContain('android:dataExtractionRules="@xml/data_extraction_rules"')
    const rules = read('./android/app/src/main/res/xml/data_extraction_rules.xml')
    expect(rules).not.toMatch(/<include\b/)
  })

  it('asks for the internet and nothing else', () => {
    const permissions = [...manifest.matchAll(/<uses-permission\s+android:name="([^"]+)"/g)].map(([, name]) => name)

    expect(permissions).toEqual(['android.permission.INTERNET'])
  })

  it('shares files only through an unexported FileProvider limited to cache/photos', () => {
    expect(manifest).toMatch(/<provider[\s\S]*?android:exported="false"[\s\S]*?<\/provider>/)
    const paths = read('./android/app/src/main/res/xml/file_paths.xml')
    const entries = [...paths.matchAll(/<([\w-]+-path)\s+name="[^"]*"\s+path="([^"]*)"/g)].map(([, kind, path]) => [kind, path])

    expect(entries).toEqual([['cache-path', 'photos/']])
  })
})

describe('the offline page', () => {
  const page = read('./www/index.html')

  it('says what happened in the shared wording and offers to try again', () => {
    expect(page).toContain('Sem conexão com o servidor. Confira a internet e tente de novo.')
    expect(page).toMatch(/<a class="retry" href="\/retry">Tentar de novo<\/a>/)
    expect(page).toContain('lang="pt-BR"')
  })

  it('links to the path the shell turns into a reload of the admin', () => {
    const policy = readFileSync(
      fileURLToPath(new URL('./android/app/src/main/java/br/com/encomendo/app/NavigationPolicy.java', import.meta.url)),
      'utf8',
    )

    expect(policy).toContain('static final String RETRY_PATH = "/retry";')
  })

  it('works with no connection: no script and nothing loaded from elsewhere', () => {
    expect(page).not.toMatch(/<script\b/)
    expect(page).not.toMatch(/\b(src|href)="(https?:)?\/\//)
  })
})
