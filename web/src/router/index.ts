import { createRouter, createWebHistory } from 'vue-router'
import OverviewView from '../views/OverviewView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [{ path: '/', name: 'overview', component: OverviewView }],
})

export default router
