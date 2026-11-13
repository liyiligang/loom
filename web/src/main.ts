import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import 'element-plus/es/components/message/style/css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import './style.css'
import App from './App.vue'
import Workspace from './views/Workspace.vue'

const router = createRouter({ history: createWebHistory(), routes: [
  { path: '/', component: Workspace },
  { path: '/sessions/:id', component: Workspace },
  { path: '/:pathMatch(.*)*', redirect: '/' },
] })
createApp(App).use(router).mount('#app')
