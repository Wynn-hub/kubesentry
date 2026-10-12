import { createRouter, createWebHistory } from 'vue-router'
import AppLayout from '../layouts/AppLayout.vue'
import { authState, can, refreshAuth, type Capability } from '../auth/session'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/setup', name: 'setup', component: () => import('../views/SetupAdminView.vue') },
    { path: '/login', name: 'login', component: () => import('../views/LoginView.vue') },
    { path: '/activate', name: 'activate', component: () => import('../views/ActivateUserView.vue') },
    { path: '/change-password', name: 'change-password', component: () => import('../views/ChangePasswordView.vue') },
    {
      path: '/',
      component: AppLayout,
      children: [
        { path: '', name: 'overview', component: () => import('../views/OverviewView.vue') },
        { path: 'policies', name: 'policies', component: () => import('../views/PolicyListView.vue') },
        { path: 'policies/new', name: 'policy-new', component: () => import('../views/PolicyFormView.vue'), meta: { capability: 'resource.write', backFallback: { name: 'policies' } } },
        { path: 'policies/:name', name: 'policy-detail', component: () => import('../views/PolicyDetailView.vue'), meta: { backFallback: { name: 'policies' } } },
        { path: 'policies/:name/edit', name: 'policy-edit', component: () => import('../views/PolicyFormView.vue'), meta: { capability: 'resource.write', backFallback: { name: 'policy-detail', inheritParams: ['name'], fallbackName: 'policies' } } },
        { path: 'policygroups', name: 'groups', component: () => import('../views/GroupListView.vue') },
        { path: 'policygroups/new', name: 'group-new', component: () => import('../views/GroupFormView.vue'), meta: { capability: 'resource.write', backFallback: { name: 'groups' } } },
        { path: 'policygroups/:name', name: 'group-detail', component: () => import('../views/GroupDetailView.vue'), meta: { backFallback: { name: 'groups' } } },
        { path: 'policygroups/:name/edit', name: 'group-edit', component: () => import('../views/GroupFormView.vue'), meta: { capability: 'resource.write', backFallback: { name: 'group-detail', inheritParams: ['name'], fallbackName: 'groups' } } },
        { path: 'exceptions', name: 'exceptions', component: () => import('../views/ExceptionListView.vue') },
        { path: 'exceptions/new', name: 'exception-new', component: () => import('../views/ExceptionFormView.vue'), meta: { capability: 'resource.write', backFallback: { name: 'exceptions' } } },
        { path: 'exceptions/:name/edit', name: 'exception-edit', component: () => import('../views/ExceptionFormView.vue'), meta: { capability: 'resource.write', backFallback: { name: 'exceptions' } } },
        { path: 'clusters', name: 'clusters', component: () => import('../views/ClusterListView.vue') },
        { path: 'clusters/:name', name: 'cluster-detail', component: () => import('../views/ClusterDetailView.vue'), meta: { backFallback: { name: 'clusters' } } },
        { path: 'system/api-docs', name: 'api-docs', component: () => import('../views/ApiDocsView.vue') },
        { path: 'tenants', name: 'tenants', component: () => import('../views/TenantListView.vue'), meta: { capability: 'platform.manage' } },
        { path: 'users', name: 'users', component: () => import('../views/UserListView.vue'), meta: { capability: 'tenant.manage' } },
        { path: 'events', name: 'events', component: () => import('../views/AdmissionEventsView.vue') },
        { path: 'observability', name: 'observability', component: () => import('../views/PlatformObservabilityView.vue') },
        { path: 'audit', name: 'audit', component: () => import('../views/ComingSoonView.vue'), meta: { feature: 'audit', backFallback: { name: 'overview' } } },
        { path: 'alerts', name: 'alerts', component: () => import('../views/ComingSoonView.vue'), meta: { feature: 'alerts', backFallback: { name: 'overview' } } },
      ],
    },
  ],
})

router.beforeEach(async (to) => {
  if (authState.status === 'unknown') await refreshAuth()
  if (authState.status === 'setup' && to.name !== 'setup') return { name: 'setup' }
  const anonymousRoute = to.name === 'login' || to.name === 'activate'
  if (authState.status === 'anonymous' && !anonymousRoute) return { name: 'login', query: { redirect: to.fullPath } }
  if (authState.status === 'authenticated' && authState.me?.mustChangePassword && to.name !== 'change-password') return { name: 'change-password' }
  if (authState.status === 'authenticated' && (to.name === 'login' || to.name === 'setup' || to.name === 'activate')) return { name: 'overview' }
  const capability = to.meta.capability as Capability | undefined
  if (authState.status === 'authenticated' && capability && !can(capability)) return { name: 'overview' }
})

export default router
