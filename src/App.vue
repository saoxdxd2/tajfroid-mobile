<template>
  <div class="app">
    <nav class="bottom-nav">
      <router-link to="/catalog">Catalog</router-link>
      <router-link to="/inventory">Inventory</router-link>
      <router-link to="/scan">Scan</router-link>
      <router-link to="/queue">Queue</router-link>
      <router-link to="/account">Account</router-link>
    </nav>
    <main>
      <router-view />
    </main>
  </div>
</template>

<style scoped>
.bottom-nav { display: flex; justify-content: space-around; padding: 8px; border-top: 1px solid #ccc; position: fixed; bottom: 0; width: 100%; background: #fff; }
main { padding-bottom: 60px; }
</style>