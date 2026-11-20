# Vue Overlay

Loaded on top of `javascript.md` (and `typescript.md`) when the component depends on `vue` or `nuxt`. Follow the app's API style: `<script setup>` + Composition API for new code unless the codebase is Options API throughout.

## Components

- Use single-file components with `<script setup lang="ts">` in TS projects. Declare props and emits with `defineProps<...>()` / `defineEmits<...>()`, and never mutate props (emit an event or use `defineModel`).
- Move reusable stateful logic into composables (`useCart()`) that take dependencies as arguments. Keep components focused on presentation.
- Use `computed` for derived state rather than watchers that copy values. Use `watch` only for side effects, and clean up in `onUnmounted`.
- Use shared state through the app's store (Pinia) with typed actions. No module-level reactive singletons outside the store.
- Use `v-for` with stable `:key`s, and never put `v-if` and `v-for` on the same element.
- Never use `v-html` with untrusted content.

## Nuxt

- Server routes (`server/api`) are backend code: validate input and check auth. Use `useFetch`/`useAsyncData` for SSR-safe data loading. Only `runtimeConfig.public` reaches the client.

## Tests

- **Vue Test Utils** or **@testing-library/vue** with Vitest. Mount the component, interact through rendered elements, and assert emitted events and visible output.
- Type-check with `vue-tsc --noEmit` where configured.
