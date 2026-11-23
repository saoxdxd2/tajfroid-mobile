<template>
  <div class="app">
    <main>
      <router-view />
    </main>
    <nav class="bottom-nav">
      <router-link to="/catalog">Catalog</router-link>
      <router-link to="/inventory">Inventory</router-link>
      <router-link to="/sales">Sales</router-link>
      <router-link to="/contacts">Contacts</router-link>
      <router-link to="/scan">Scan</router-link>
      <router-link to="/queue">Queue</router-link>
      <router-link to="/account">Account</router-link>
    </nav>
  </div>
</template>

<style scoped>
main { padding-bottom: 120px; }
</style>
