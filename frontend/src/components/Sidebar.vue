<script setup>
import { computed } from 'vue'
import { useTheme } from '../composables/useTheme'

const props = defineProps({
  // Which destination is highlighted. The game page passes 'library', since it
  // is reached from there and the sidebar should not appear to leave it.
  active: { type: String, required: true },
})

defineEmits(['navigate'])

const { theme, toggleTheme } = useTheme()

// The design's third destination, Queue, is deliberately absent: there is no
// queue behind it. InstallVersion refuses a second concurrent job, and that is
// the accepted behaviour rather than a stopgap.
const items = [
  { id: 'library', label: 'Library' },
  { id: 'roms', label: 'ROMs' },
  { id: 'settings', label: 'Settings' },
]

// Names the mode the toggle switches *to*, not the one in force.
const themeLabel = computed(() =>
  theme.value === 'dark' ? 'Light mode' : 'Dark mode'
)
</script>

<template>
  <nav class="sidebar" aria-label="Main">
    <div class="wordmark">PortForge</div>

    <ul class="nav">
      <li v-for="item in items" :key="item.id">
        <button
          class="nav-item"
          :class="{ active: props.active === item.id }"
          :aria-current="props.active === item.id ? 'page' : undefined"
          @click="$emit('navigate', item.id)"
        >{{ item.label }}</button>
      </li>
    </ul>

    <div class="sidebar-spacer" />

    <button class="theme-toggle" @click="toggleTheme">{{ themeLabel }}</button>
  </nav>
</template>

<style lang="scss" scoped>
.sidebar {
  flex: 0 0 196px;
  display: flex;
  flex-direction: column;
  padding: 22px 14px 14px;
  background: var(--panel);
  border-right: 1px solid var(--line);
  z-index: 3;
}

/* The cartridge mark stays the window icon; the sidebar is text only. */
.wordmark {
  padding: 0 10px 22px;
  font-size: 17px;
  font-weight: 600;
  letter-spacing: -0.015em;
  color: var(--text);
}

.nav {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.nav-item {
  width: 100%;
  padding: 9px 10px;
  border-radius: var(--r-control);
  text-align: left;
  font-size: 13.5px;
  font-weight: 500;
  color: var(--dim);
  transition: var(--t-bg);

  &:hover {
    background: var(--panel2);
  }

  &.active {
    background: var(--panel2);
    color: var(--text);
    font-weight: 600;
  }
}

.sidebar-spacer {
  flex: 1;
}

.theme-toggle {
  padding: 9px 10px;
  border-radius: var(--r-control);
  text-align: left;
  font-size: 12.5px;
  color: var(--dim);
  transition: var(--t-bg);

  &:hover {
    background: var(--panel2);
    color: var(--text);
  }
}
</style>
