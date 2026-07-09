import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import './styles/tokens.css'
import App from './App.vue'
import router from './router'
import { i18n } from './locales'

createApp(App).use(router).use(i18n).use(ElementPlus).mount('#app')
