import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import i18n from './i18n'
import { initTheme } from './lib/theme'
import './style.css'

initTheme()

createApp(App).use(router).use(i18n).mount('#app')
