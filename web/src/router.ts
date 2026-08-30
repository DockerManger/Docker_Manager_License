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
        {
          path: '',
          redirect: '/dashboard',
        },
        {
          path: 'dashboard',
          name: 'dashboard',
          component: () => import('./views/DashboardView.vue'),
          meta: { titleKey: 'dashboard.title', descKey: 'dashboard.description' },
        },
        {
          path: 'licenses',
          name: 'licenses',
          component: () => import('./views/LicensesView.vue'),
          meta: { titleKey: 'licenses.title', descKey: 'licenses.description' },
        },
        {
          path: 'licenses/:id',
          name: 'license-detail',
          component: () => import('./views/LicenseDetailView.vue'),
          meta: { titleKey: 'detail.title', descKey: 'detail.desc' },
        },
        {
          path: 'customers',
          name: 'customers',
          component: () => import('./views/CustomersView.vue'),
          meta: { titleKey: 'customers.title', descKey: 'customers.description' },
        },
        {
          path: 'subscriptions',
          name: 'subscriptions',
          component: () => import('./views/SubscriptionsView.vue'),
          meta: { titleKey: 'subscriptions.title', descKey: 'subscriptions.description' },
        },
        {
          path: 'security',
          name: 'security',
          component: () => import('./views/SecurityEventsView.vue'),
          meta: { titleKey: 'security.title', descKey: 'security.description' },
        },
        {
          path: 'audit',
          name: 'audit',
          component: () => import('./views/AuditLogsView.vue'),
          meta: { titleKey: 'audit.title', descKey: 'audit.description' },
        },
        {
          path: 'settings',
          name: 'settings',
          component: () => import('./views/SettingsView.vue'),
          meta: { titleKey: 'settings.title', descKey: 'settings.description' },
        },
      ],
    },
  ],
})

router.beforeEach((to) => {
  if (to.name !== 'login' && !getToken()) return { name: 'login' }
  if (to.name === 'login' && getToken()) return { name: 'dashboard' }
})

export default router
