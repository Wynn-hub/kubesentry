import { createRouter, createWebHistory } from 'vue-router'
import AppLayout from '../layouts/AppLayout.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/',
      component: AppLayout,
      children: [
        { path: '', name: 'overview', component: () => import('../views/OverviewView.vue') },
        { path: 'policies', name: 'policies', component: () => import('../views/PolicyListView.vue') },
        { path: 'policies/new', name: 'policy-new', component: () => import('../views/PolicyFormView.vue') },
        { path: 'policies/:name', name: 'policy-detail', component: () => import('../views/PolicyDetailView.vue') },
        { path: 'policies/:name/edit', name: 'policy-edit', component: () => import('../views/PolicyFormView.vue') },
        { path: 'policygroups', name: 'groups', component: () => import('../views/GroupListView.vue') },
        { path: 'policygroups/new', name: 'group-new', component: () => import('../views/GroupFormView.vue') },
        { path: 'policygroups/:name', name: 'group-detail', component: () => import('../views/GroupDetailView.vue') },
        { path: 'policygroups/:name/edit', name: 'group-edit', component: () => import('../views/GroupFormView.vue') },
        { path: 'exceptions', name: 'exceptions', component: () => import('../views/ExceptionListView.vue') },
        { path: 'exceptions/new', name: 'exception-new', component: () => import('../views/ExceptionFormView.vue') },
        { path: 'exceptions/:name/edit', name: 'exception-edit', component: () => import('../views/ExceptionFormView.vue') },
      ],
    },
  ],
})

export default router
