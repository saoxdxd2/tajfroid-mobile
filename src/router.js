import { createRouter, createWebHistory } from 'vue-router'
import PairView from './views/PairView.vue'
import CatalogView from './views/CatalogView.vue'
import InventoryView from './views/InventoryView.vue'
import CustomersView from './views/CustomersView.vue'
import SuppliersView from './views/SuppliersView.vue'
import ScanView from './views/ScanView.vue'
import QueueView from './views/QueueView.vue'
import AccountView from './views/AccountView.vue'

const routes = [
  { path: '/', redirect: '/catalog' },
  { path: '/pair', component: PairView },
  { path: '/catalog', component: CatalogView },
  { path: '/inventory', component: InventoryView },
  { path: '/customers', component: CustomersView },
  { path: '/suppliers', component: SuppliersView },
  { path: '/scan', component: ScanView },
  { path: '/queue', component: QueueView },
  { path: '/account', component: AccountView },
]

export default createRouter({ history: createWebHistory(), routes })