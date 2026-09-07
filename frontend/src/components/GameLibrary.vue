<script setup>
import { ref, computed, inject } from 'vue'
import { artworkUrl, ART_WIDTH } from '../lib/artwork'

const props = defineProps({
  versions: { type: Array, required: true },
  // itemTitle → LibraryStatus from the Go side. Absent until the first load
  // resolves, in which case cards fall back to "Not installed".
  status: { type: Object, default: () => ({}) },
})

defineEmits(['select'])

const activeInstall = inject('activeInstall', ref(null))

const search = ref('')
const filter = ref('all')

const FILTERS = [
  { id: 'all', label: 'All ports' },
  { id: 'installed', label: 'Installed' },
  { id: 'updates', label: 'Updates' },
  { id: 'buildable', label: 'Ready to build' },
  { id: 'missing-rom', label: 'Missing ROM' },
]

function coverUrl(version) {
  const art = version.artwork?.find(a => a.artworkType.toLowerCase() === 'cover')
    ?? version.artwork?.[0]
  if (!art) return null
  return artworkUrl(version, art.fileName, ART_WIDTH.gridCover)
}

function statusOf(version) {
  return props.status[version._itemTitle] ?? {}
}

// The base game line under the title: "Ocarina of Time · 1998".
function baseGameLine(version) {
  const g = version.videoGame
  if (!g) return ''
  const title = g.title || g._itemTitle
  return g.releaseYear ? `${title} · ${g.releaseYear}` : title
}

// One of: busy | update | installed | missing-rom | buildable | unavailable.
// Ordered by what the user most needs to know — an install in flight outranks
// everything, and a missing ROM outranks "ready" because it is the blocker.
function cardState(version) {
  if (activeInstall.value?.itemTitle === version._itemTitle) return 'busy'
  const st = statusOf(version)
  if (st.hasUpdate) return 'update'
  if (st.installed) return 'installed'
  if (st.hasRomDeps && !st.romsReady) return 'missing-rom'
  if (st.buildable) return 'buildable'
  return 'unavailable'
}

const STATE_LABELS = {
  update: 'Update ready',
  installed: 'Installed',
  'missing-rom': 'ROM needed',
  buildable: 'Not installed',
  unavailable: 'Unavailable',
}

function stateLabel(version) {
  const state = cardState(version)
  if (state !== 'busy') return STATE_LABELS[state]
  const phase = activeInstall.value?.phase
  const verb = phase === 'downloading' ? 'Downloading'
    : phase === 'extracting' ? 'Extracting'
    : 'Building'
  // Off the artwork there is room for the number, and the 3px bar underneath is
  // too thin to read a value off on its own.
  const pct = busyPercent(version)
  return pct > 0 ? `${verb} · ${pct}%` : verb
}

// The version shown in the status row: what is installed, else what would be built.
function versionLabel(version) {
  const st = statusOf(version)
  return st.installedVersion || st.latestVersion || ''
}

function busyPercent(version) {
  if (cardState(version) !== 'busy') return 0
  const { phase, percent, stepIndex, stepTotal } = activeInstall.value
  if (phase === 'done') return 100
  if (stepTotal > 0) return Math.round(((stepIndex + 1) / stepTotal) * 100)
  if (phase === 'downloading') return Math.round((percent ?? 0) * 0.8)
  return 0
}

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  return props.versions.filter(v => {
    if (q) {
      const haystack = [
        v.title || v._itemTitle,
        v.videoGame?.title || v.videoGame?._itemTitle || '',
      ].join(' ').toLowerCase()
      if (!haystack.includes(q)) return false
    }
    const st = statusOf(v)
    switch (filter.value) {
      case 'installed': return !!st.installed
      case 'updates': return !!st.hasUpdate
      case 'buildable': return !!st.buildable && !st.installed
      case 'missing-rom': return !!st.hasRomDeps && !st.romsReady
      default: return true
    }
  })
})

const countLine = computed(() => {
  const total = props.versions.length
  const installed = props.versions.filter(v => statusOf(v).installed).length
  return `${total} port${total === 1 ? '' : 's'} · ${installed} installed`
})
</script>

<template>
  <div class="library">
    <header class="library-header">
      <h1 class="page-title">Library</h1>
      <span class="library-count">{{ countLine }}</span>
      <div class="library-spacer" />
      <label class="search">
        <span class="search-glyph" aria-hidden="true" />
        <input v-model="search" type="search" placeholder="Search ports" aria-label="Search ports" />
      </label>
    </header>

    <div class="library-body">
      <div class="chips" role="group" aria-label="Filter ports">
        <button
          v-for="f in FILTERS"
          :key="f.id"
          class="chip"
          :class="{ active: filter === f.id }"
          :aria-pressed="filter === f.id"
          @click="filter = f.id"
        >{{ f.label }}</button>
      </div>

      <p v-if="!versions.length" class="library-empty">
        No ports in the catalog yet. Use <strong>Refresh catalog</strong> in Settings to fetch them.
      </p>

      <p v-else-if="!filtered.length" class="library-empty">
        Nothing matches this filter.
        <button class="link-btn" @click="search = ''; filter = 'all'">Clear filters</button>
      </p>

      <ul v-else class="grid">
        <li v-for="version in filtered" :key="version._itemTitle">
          <button class="cover-card" @click="$emit('select', version)">
            <div class="cover" :class="{ 'art-placeholder': !coverUrl(version) }">
              <img
                v-if="coverUrl(version)"
                :src="coverUrl(version)"
                :alt="version.title || version._itemTitle"
              />
              <span v-else class="art-placeholder__label">COVER</span>
            </div>

            <span class="card-title">{{ version.title || version._itemTitle }}</span>
            <span v-if="baseGameLine(version)" class="card-subtitle">{{ baseGameLine(version) }}</span>

            <span class="status-row">
              <span class="dot" :class="cardState(version)" />
              <span class="status-label" :class="cardState(version)">{{ stateLabel(version) }}</span>
              <span class="status-spacer" />
              <span v-if="versionLabel(version)" class="status-version mono">
                {{ versionLabel(version) }}
              </span>
            </span>

            <span v-if="cardState(version) === 'busy'" class="card-progress">
              <span class="card-progress-fill" :style="{ width: busyPercent(version) + '%' }" />
            </span>
          </button>
        </li>
      </ul>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.library {
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.library-header {
  position: sticky;
  top: 0;
  z-index: 2;
  display: flex;
  align-items: center;
  gap: 18px;
  padding: 18px var(--pad-page) 14px;
  background: var(--bg);
  border-bottom: 1px solid var(--line);
  flex-shrink: 0;
}

.library-count {
  font-size: 13px;
  color: var(--dim2);
}

.library-spacer {
  flex: 1;
}

.search {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 230px;
  padding: 7px 11px;
  background: var(--panel);
  border: 1px solid var(--line);
  border-radius: var(--r-control);
  color: var(--dim2);

  &:focus-within {
    border-color: var(--line2);
  }

  input {
    flex: 1;
    min-width: 0;
    border: none;
    outline: none;
    background: none;
    font-size: 13px;
    color: var(--text);

    &::placeholder {
      color: var(--dim2);
    }

    // The native clear affordance does not match the rest of the chrome.
    &::-webkit-search-cancel-button {
      appearance: none;
    }
  }
}

.search-glyph {
  width: 12px;
  height: 12px;
  border: 2px solid currentColor;
  border-radius: 50%;
  flex-shrink: 0;
}

.library-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 18px var(--pad-page) 44px;
}

.chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 22px;
}

.chip {
  padding: 6px 13px;
  border-radius: var(--r-pill);
  border: 1px solid var(--line);
  background: var(--panel);
  color: var(--dim);
  font-size: 12.5px;
  font-weight: 500;
  transition: var(--t-bg);

  &:hover {
    color: var(--text);
  }

  &.active {
    background: var(--accent-soft);
    border-color: var(--line2);
    color: var(--accent-hi);
  }
}

.library-empty {
  margin-top: 64px;
  text-align: center;
  font-size: 13.5px;
  color: var(--dim2);
}

.link-btn {
  color: var(--accent);
  font-size: 13.5px;
  font-weight: 500;

  &:hover {
    color: var(--accent-hi);
  }
}

.grid {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(186px, 1fr));
  gap: 26px 22px;
  align-items: start;
}

.cover-card {
  display: flex;
  flex-direction: column;
  width: 100%;
  text-align: left;
  color: inherit;
}

/* Every tile is 2:3 — the shape of the catalog's art — so the titles and status
   rows beneath them line up across a row. Art that is not 2:3 is letterboxed
   (`contain`) rather than cropped: the grid stays uniform without any cover
   losing its edges. The bars are painted by the image's own background, which
   leaves the striped placeholder's background-color alone. */
.cover {
  position: relative;
  width: 100%;
  aspect-ratio: 2 / 3;
  border: 1px solid var(--line);
  border-radius: var(--r-cover);
  box-shadow: var(--shadow);
  overflow: hidden;
  /* Deliberately not animated on hover. Lifting the card is cheap on Chromium and
     WKWebView but not on WebKitGTK, where moving the pointer across a full grid
     is the most visible stutter in the app — and the grid is the view people
     spend the most time in. */

  img {
    width: 100%;
    height: 100%;
    object-fit: contain;
    background: var(--panel2);
    display: block;
  }
}

.card-title {
  margin-top: 11px;
  font-size: 14px;
  font-weight: 500;
  letter-spacing: -0.005em;
  color: var(--text);
  text-wrap: pretty;
}

.card-subtitle {
  margin-top: 3px;
  font-size: 12px;
  color: var(--dim2);
  text-wrap: pretty;
}

/* Below the art rather than on a scrim over it: box art gets to be box art, and
   the label is legible without fighting whatever is underneath it. */
.status-row {
  margin-top: 8px;
  display: flex;
  align-items: center;
  gap: 7px;
}

.dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  flex-shrink: 0;
  background: var(--dim2);

  &.installed { background: var(--ok); }
  &.update { background: var(--warn); }
  &.busy { background: var(--accent); }
  &.missing-rom { background: var(--bad); }
}

/* Only the states that need attention colour their label; "Installed" is the
   resting state and stays quiet next to its green dot. */
.status-label {
  font-size: 11.5px;
  font-weight: 500;
  color: var(--dim2);
  white-space: nowrap;

  &.installed { color: var(--dim); }
  &.update { color: var(--warn); }
  &.busy { color: var(--accent); }
  &.missing-rom { color: var(--bad); }
}

.status-spacer {
  flex: 1;
}

.status-version {
  font-size: 10.5px;
  color: var(--dim2);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 45%;
}

.card-progress {
  display: block;
  margin-top: 8px;
  height: 3px;
  border-radius: 3px;
  background: var(--panel2);
  overflow: hidden;
}

.card-progress-fill {
  display: block;
  height: 100%;
  background: var(--accent);
  transition: width 200ms ease;
}
</style>
