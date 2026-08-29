import { createRouter, createWebHistory } from 'vue-router'
import { getToken } from './api'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: () => import('./views/LoginView.vue') },
    {
      path: '/',
      component: () => import('./layouts/MainLayout.vue'),
      children: [
        { path: '', redirect: '/dashboard' },
        { path: 'dashboard', name: 'dashboard', component: () => import('./views/DashboardView.vue') },
        { path: 'licenses', name: 'licenses', component: () => import('./views/LicensesView.vue') },
        { path: 'licenses/:id', name: 'license-detail', component: () => import('./views/LicenseDetailView.vue') },
        { path: 'audit', name: 'audit', component: () => import('./views/AuditLogsView.vue') },
        { path: 'settings', name: 'settings', component: () => import('./views/SettingsView.vue') },
      ],
    },
  ],
})

router.beforeEach((to) => {
  if (to.name !== 'login' && !getToken()) return { name: 'login' }
  if (to.name === 'login' && getToken()) return { name: 'dashboard' }
})

export default router
