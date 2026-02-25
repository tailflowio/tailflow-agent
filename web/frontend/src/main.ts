import { createApp } from 'vue'
import { createPinia } from 'pinia'
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
      path: '/steps',
      name: 'steps',
      component: () => import('./views/StepsListView.vue'),
    },
    {
      path: '/steps/:id',
      name: 'step-detail',
      component: () => import('./views/StepDetailView.vue'),
    },
  ],
})

const app = createApp(App)
app.use(createPinia())
app.use(router)
app.use(i18n as any)
app.mount('#app')
