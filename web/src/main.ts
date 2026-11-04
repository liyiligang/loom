import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import { ElDialog } from 'element-plus'
import 'element-plus/es/components/dialog/style/css'
import 'element-plus/es/components/message/style/css'
import './style.css'
import App from './App.vue'
import Workspace from './views/Workspace.vue'

const router = createRouter({ history: createWebHistory(), routes: [
  { path: '/', component: Workspace },
  { path: '/sessions/:id', component: Workspace },
  { path: '/:pathMatch(.*)*', redirect: '/' },
] })
createApp(App).use(router).component('ElDialog', ElDialog).mount('#app')
