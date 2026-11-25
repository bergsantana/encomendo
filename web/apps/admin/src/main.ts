import '@encomendo/ui/tokens.css'
import '@encomendo/ui/fonts'

import { createPinia } from 'pinia'
import { createApp } from 'vue'

import App from './App.vue'
import { reloadWhenRestoredFromCache } from './pageLoad'
import { createAppRouter } from './router'

reloadWhenRestoredFromCache()

// Pinia first: the router's first navigation runs the auth guard, which
// reads the session store.
createApp(App).use(createPinia()).use(createAppRouter()).mount('#app')
