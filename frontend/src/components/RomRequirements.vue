<script setup>
// One port's ROM requirements, rendered the same way everywhere they appear.
//
// The game page and the ROM library both show this list, and they must look
// identical: a row that reads differently in the two places invites the user to
// wonder which one is telling the truth. The rules behind it live in
// useRomRequirements for the same reason.
import { ref, watch } from 'vue'
import { useRomRequirements } from '../composables/useRomRequirements'

const props = defineProps({
  requirements: { type: Array, required: true },
  // MD5 -> present. Supplied by the caller because the game page scopes it to
  // one port while the ROM library resolves every port in one pass.
  status: { type: Object, required: true },
  // The "Add file" row. The game page shows it; the ROM library puts its own
  // button in each port's header instead, where it can name the port.
  footer: { type: Boolean, default: false },
  adding: { type: Boolean, default: false },
  // Open every unsatisfied group on first render, so the filenames and
  // checksums needed to go and find a missing dump are visible without a
  // second click. The game page wants this — it is about one port. The ROM
  // library does not: opening every unmet group across nine ports would bury
  // the list it is meant to summarise.
  expandUnmet: { type: Boolean, default: false },
})

const emit = defineEmits(['add'])

const expanded = ref({})

const {
  optionPresent, formatPresent, requirementMet, sortedOptions,
} = useRomRequirements(() => props.status)

function keyFor(req, i) {
  return req.name || String(i)
}

function toggle(key) {
  expanded.value = { ...expanded.value, [key]: !expanded.value[key] }
}

// A lone unnamed requirement has nothing to head it: the dumps beneath it are
// the whole story, so there is no group to open and they are simply shown.
function hasHead(req) {
  return props.requirements.length > 1 || !!req.name
}

function isOpen(req, i) {
  return !hasHead(req) || !!expanded.value[keyFor(req, i)]
}

// Runs once per set of requirements, never on a status change: re-expanding a
// row the user had collapsed because a scan finished elsewhere would undo their
// action for no reason. Waiting for a non-empty status map avoids expanding
// everything in the moment before presence is known.
let autoExpandedFor = null
watch(
  () => [props.requirements, props.status],
  () => {
    if (!props.expandUnmet) return
    if (autoExpandedFor === props.requirements) return
    if (!Object.keys(props.status ?? {}).length) return
    autoExpandedFor = props.requirements

    const next = {}
    props.requirements?.forEach((req, i) => {
      if (!requirementMet(req)) next[keyFor(req, i)] = true
    })
    expanded.value = next
  },
  { immediate: true },
)

function romTypeLabel(type) {
  const labels = {
    N64CartRom: 'N64',
    NESCartRom: 'NES',
    GBCartRom: 'Game Boy',
    GBCCartRom: 'Game Boy Color',
    PS1DiscImage: 'PS1',
    Xbox360DiscImage: 'Xbox 360',
  }
  return labels[type] ?? String(type ?? '').replace(/(CartRom|DiscImage|Rom)$/, '')
}

function formatSize(bytes) {
  if (!bytes) return '—'
  if (bytes >= 1073741824) return (bytes / 1073741824).toFixed(1) + ' GB'
  if (bytes >= 1048576) return (bytes / 1048576).toFixed(1) + ' MB'
  if (bytes >= 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return bytes + ' B'
}
</script>

<template>
  <div class="card rom-list">
    <template v-for="(req, i) in requirements" :key="keyFor(req, i)">
      <!-- The requirement is the disclosure, not each dump. Its options are
           alternatives to each other, so what a reader wants once they open one
           is all of them at once to check against what they have - not one more
           click per row. -->
      <button
        v-if="hasHead(req)"
        class="rom-req-head"
        :class="{ open: isOpen(req, i) }"
        :aria-expanded="isOpen(req, i)"
        @click="toggle(keyFor(req, i))"
      >
        <span v-if="requirementMet(req)" class="icon-check" aria-hidden="true" />
        <span v-else-if="req.required" class="icon-cross-sm" aria-hidden="true" />
        <span v-else class="rom-req-dot" aria-hidden="true" />
        <span class="rom-req-label">{{ req.name || 'Game ROM' }}</span>
        <span class="rom-req-count">
          {{ req.options?.length === 1 ? '1 dump' : `${req.options?.length ?? 0} dumps` }}
        </span>
        <span class="rom-req-flag">{{ req.required ? 'required' : 'adds content' }}</span>
        <span class="chevron-down rom-caret" :class="{ open: isOpen(req, i) }" aria-hidden="true" />
      </button>

      <template v-if="isOpen(req, i)">
        <div v-for="dep in sortedOptions(req)" :key="dep.title" class="rom-dep">
          <div class="rom-dep-head">
            <span v-if="optionPresent(dep) === true" class="icon-check" aria-hidden="true" />
            <span v-else class="icon-cross-sm" aria-hidden="true" />
            <span class="rom-dep-title mono" :class="{ missing: optionPresent(dep) !== true }">
              {{ dep.title }}
            </span>
            <span class="rom-dep-type">{{ romTypeLabel(dep._itemType) }}</span>
          </div>

          <div class="rom-formats">
            <p v-if="!dep.formats?.length" class="rom-formats-empty">
              No file details in the catalog for this ROM.
            </p>
            <div v-for="fmt in dep.formats" :key="fmt.checksums.md5" class="rom-format">
              <div class="rom-format-line">
                <span class="mono rom-format-name">{{ fmt.filename }}</span>
                <span class="rom-format-meta">{{ fmt.ext }} &middot; {{ formatSize(fmt.filesize) }}</span>
                <span v-if="formatPresent(fmt)" class="icon-check" aria-hidden="true" />
              </div>
              <p v-if="fmt.checksums.md5" class="mono checksum">MD5 {{ fmt.checksums.md5 }}</p>
              <p v-if="fmt.checksums.sha1" class="mono checksum">SHA1 {{ fmt.checksums.sha1 }}</p>
            </div>
          </div>
        </div>
      </template>
    </template>

    <div v-if="footer" class="rom-foot">
      <button class="btn-outline" :disabled="adding" @click="emit('add')">
        {{ adding ? 'Adding&hellip;' : 'Add file&hellip;' }}
      </button>
      <span class="rom-foot-note">Files are matched by checksum, never uploaded.</span>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.rom-req-head {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 9px 16px;
  text-align: left;
  border-bottom: 1px solid var(--line);
  background: var(--panel2);

  &:hover { background: var(--panel3); }
}

.rom-req-label { flex: 1; font-size: 12px; color: var(--text); }

.rom-req-count {
  font-size: 11px;
  color: var(--dim2);
  white-space: nowrap;
}

.rom-req-flag {
  font-size: 10.5px;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: var(--dim);
}

.rom-req-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--dim);
  opacity: 0.5;
}

.rom-list { padding: 0; overflow: hidden; }

.rom-dep { border-bottom: 1px solid var(--line); }

.rom-dep-head {
  display: flex;
  align-items: center;
  gap: 13px;
  width: 100%;
  padding: 11px 16px 3px;
  color: var(--dim);
}

.rom-dep-title {
  flex: 1;
  min-width: 0;
  font-size: 12px;
  line-height: 1.45;
  color: var(--text);
  text-wrap: pretty;

  &.missing { color: var(--dim2); }
}

.rom-dep-type {
  font-size: 11px;
  color: var(--dim2);
  flex-shrink: 0;
}

.rom-caret { color: var(--dim2); }

.rom-formats {
  padding: 2px 16px 12px 47px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.rom-formats-empty { margin: 0; font-size: 12px; color: var(--dim2); }

.rom-format-line {
  display: flex;
  align-items: center;
  gap: 10px;
}

.rom-format-name {
  flex: 1;
  min-width: 0;
  font-size: 11.5px;
  color: var(--text);
  word-break: break-all;
}

.rom-format-meta {
  font-size: 11.5px;
  color: var(--dim2);
  white-space: nowrap;
}

.checksum {
  margin: 3px 0 0;
  font-size: 10.5px;
  color: var(--dim2);
  word-break: break-all;
}

.rom-foot {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 13px 16px;
}

.rom-foot-note { font-size: 12.5px; color: var(--dim2); }

/* The last row's border would otherwise double with the card's own edge. */
.rom-dep:last-child { border-bottom: none; }
</style>
