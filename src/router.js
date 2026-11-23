import { createRouter, createWebHistory } from 'vue-router'
import PairView from './views/PairView.vue'
import CatalogView from './views/CatalogView.vue'
import InventoryView from './views/InventoryView.vue'
import SalesView from './views/SalesView.vue'
import InvoiceDetailView from './views/InvoiceDetailView.vue'
import PurchasesView from './views/PurchasesView.vue'
import PurchaseDetailView from './views/PurchaseDetailView.vue'
import FinanceView from './views/FinanceView.vue'
import ScansView from './views/ScansView.vue'
import ContactsView from './views/ContactsView.vue'
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
  { path: '/sales', component: SalesView },
  { path: '/sales/:id', component: InvoiceDetailView },
  { path: '/purchases', component: PurchasesView },
  { path: '/purchases/:id', component: PurchaseDetailView },
  { path: '/finance', component: FinanceView },
  { path: '/scans', component: ScansView },
  { path: '/contacts', component: ContactsView },
  { path: '/customers', component: CustomersView },
  { path: '/suppliers', component: SuppliersView },
  { path: '/scan', component: ScanView },
  { path: '/queue', component: QueueView },
  { path: '/account', component: AccountView },
]

export default createRouter({ history: createWebHistory(), routes })
