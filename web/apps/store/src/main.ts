import '@encomendo/ui/tokens.css'
import '@encomendo/ui/fonts'

import { createApp } from 'vue'

import App from './App.vue'
import { resolveSite } from './site'

const baseDomain = import.meta.env.VITE_STORE_BASE_DOMAIN ?? 'encomendo.com.br'
const site = resolveSite(window.location.host, baseDomain)

createApp(App, { site }).mount('#app')
