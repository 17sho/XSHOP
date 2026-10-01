<script setup lang="ts">
import { RouterView } from 'vue-router'
</script>

<template>
  <RouterView v-slot="{ Component, route: viewRoute }">
    <!-- Path-only identity preserves drafts and motion on query/hash updates. -->
    <div :key="viewRoute.path" class="min-w-0 admin-route-content" data-admin-route-view>
      <component :is="Component" />
    </div>
  </RouterView>
</template>

<style scoped>
.admin-route-content {
  /* No fill mode: release the transform after entry for fixed descendants. */
  animation: admin-route-enter 160ms ease-out;
}

@keyframes admin-route-enter {
  from {
    opacity: 0;
    transform: translateY(6px);
  }
  to {
    opacity: 1;
    transform: none;
  }
}
@media (prefers-reduced-motion: reduce) {
  .admin-route-content {
    animation: none;
  }
}
</style>
