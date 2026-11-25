/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** Domain the store subdomains hang from: encomendo.com.br in production, localhost locally. Shown when a store is created. */
  readonly VITE_STORE_BASE_DOMAIN?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
