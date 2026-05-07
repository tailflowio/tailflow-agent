import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import { i18n } from './i18n'
import App from './App.vue'
import './style.css'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      name: 'overview',
      component: () => import('./views/DashboardView.vue'),
    },
    {
      path: '/executions',
      name: 'executions',
      component: () => import('./views/ExecutionsView.vue'),
    },
    {
      path: '/executions/:id',
      name: 'execution',
      component: () => import('./views/ExecutionView.vue'),
    },
    {
      path: '/docs',
      name: 'docs',
      component: () => import('./views/DocsView.vue'),
    },
    {
      path: '/editor',
      name: 'editor',
      component: () => import('./views/EditorView.vue'),
    },
  ],
})

const app = createApp(App)
app.use(router)
app.use(i18n as any)
app.mount('#app')
