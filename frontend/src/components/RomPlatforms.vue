<script setup>
import { computed } from 'vue'

const props = defineProps({
  roms:   { type: Array,  required: true },
  status: { type: Object, required: true },
})

const emit = defineEmits(['select'])

const gamingPlatforms = [
  {
    title:         'Nintendo Entertainment System',
    logoPath:      '/platforms/nes-logo.png',
    backdropPath:  '/platforms/nes-banner.png',
    gamesItemType: 'NESRom',
  }, {
    title:         'Nintendo 64',
    logoPath:      '/platforms/n64-logo.png',
    backdropPath:  '/platforms/n64-banner.png',
    gamesItemType: 'N64Rom',
  },
]

function romCount(platform) {
  return props.roms.filter(
    r => r._itemType === platform.gamesItemType && props.status[r._itemTitle]
  ).length
}
</script>

<template>
  <div class="platforms">
    <ul class="platform-list">
      <li v-for="platform in gamingPlatforms" :key="platform.gamesItemType">
        <button class="platform-card" @click="emit('select', platform)">

          <!-- 16:9 backdrop — zooms in on hover, clipped by card overflow:hidden -->
          <div class="platform-backdrop">
            <img
              v-if="platform.backdropPath"
              :src="platform.backdropPath"
              :alt="platform.title"
            />
            <div v-else class="platform-backdrop-placeholder" />
          </div>

          <!-- logo centred over the backdrop -->
          <div class="platform-logo">
            <img
              v-if="platform.logoPath"
              :src="platform.logoPath"
              :alt="platform.title"
            />
            <span v-else class="platform-name-fallback">{{ platform.title }}</span>
          </div>

          <!-- footer strip with title + count -->
          <!-- <div class="platform-footer">
            <span class="platform-title">{{ platform.title }}</span>
            <span class="platform-count">{{ romCount(platform) }}</span>
          </div> -->

        </button>
      </li>
    </ul>
  </div>
</template>

<style lang="scss" scoped>
.platforms {
  padding: 24px;
  height: 100%;
}

.platform-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 12px;
  align-items: start;
}

/* ── Card ── */
.platform-card {
  position: relative;
  width: 100%;
  aspect-ratio: 16 / 9;
  overflow: hidden;
  border-radius: 8px;
  border: 1px solid #4c4c4c;
  cursor: pointer;
  background: #1e1e1e;
  padding: 0;
  font: inherit;
  color: inherit;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: border-color 0.15s;

  &:hover {
    border-color: #8b929a;

    .platform-backdrop {
      transform: scale(1.05);
    }
  }
}

/* ── Backdrop ── */
.platform-backdrop {
  position: absolute;
  inset: 0;
  transition: transform 0.35s ease;

  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    object-position: center;
  }
}

.platform-backdrop-placeholder {
  width: 100%;
  height: 100%;
  background: linear-gradient(135deg, #2a2a2a 0%, #1a1a1a 100%);
}

/* ── Logo ── */
.platform-logo {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  width: 100%;
  height: 100%;

  img {
    max-width: 60%;
    max-height: 55%;
    object-fit: contain;
    // filter: drop-shadow(0 2px 8px rgba(0, 0, 0, 0.7));
  }
}

.platform-name-fallback {
  font-size: 18px;
  font-weight: 700;
  color: #e8eaed;
  text-align: center;
  text-shadow: 0 2px 8px rgba(0, 0, 0, 0.8);
}

/* ── Footer ── */
.platform-footer {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  z-index: 2;
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 8px;
  padding: 10px 14px;
  // background: linear-gradient(to top, rgba(0, 0, 0, 0.75) 0%, transparent 50%);
}

.platform-title {
  font-size: 13px;
  font-weight: 600;
  color: #e8eaed;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.platform-count {
  font-size: 11px;
  color: #8b929a;
  flex-shrink: 0;

  &::after { content: ' ROMs'; }
}
</style>
