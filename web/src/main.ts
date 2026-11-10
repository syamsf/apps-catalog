import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import { loadPortalName } from './session'
import './style.css'

loadPortalName()
createApp(App).use(router).mount('#app')
