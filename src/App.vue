<template>
  <div class="app">
    <main>
      <router-view />
    </main>
    <nav class="bottom-nav">
      <router-link to="/catalog">Catalog</router-link>
      <router-link to="/inventory">Inventory</router-link>
      <router-link to="/sales">Sales</router-link>
      <router-link to="/purchases">Purchases</router-link>
      <router-link to="/contacts">Contacts</router-link>
      <router-link to="/finance">Finance</router-link>
      <router-link to="/scans">Scans</router-link>
      <router-link to="/account">Account</router-link>
    </nav>
  </div>
</template>

<style scoped>
main { padding-bottom: 150px; }
</style>
