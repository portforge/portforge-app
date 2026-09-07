<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { GetROMLibrary, SelectROMFiles, AddROMFiles } from '../../wailsjs/go/main/App'
import { useRomRequirements } from '../composables/useRomRequirements'
import RomRequirements from './RomRequirements.vue'

const props = defineProps({
  // Bumped by App.vue whenever ROMs are imported, so the view reflects a drop
  // that happened while it was open.
  refreshKey: { type: Number, default: 0 },
})

const library = ref({ ports: [], status: {} })
const loading = ref(true)
const error = ref(null)
const addingFor = ref(null)
const addError = ref(null)

const { optionPresent, requirementMet } = useRomRequirements(() => library.value.status)

async function load() {
  error.value = null
  try {
    library.value = (await GetROMLibrary()) ?? { ports: [], status: {} }
  } catch (e) {
    error.value = String(e)
  } finally {
    loading.value = false
  }
}

onMounted(load)
watch(() => props.refreshKey, load)

// Every dump this catalog knows about, deduplicated: two ports accepting the
// same ROM is one file on disk, and counting it twice would overstate both the
// total and what is missing.
const totals = computed(() => {
  const seen = new Map()
  for (const port of library.value.ports ?? []) {
    for (const req of port.romDependencies ?? []) {
      for (const opt of req.options ?? []) {
        const key = `${opt._itemType} ${opt.title}`
        if (!seen.has(key)) seen.set(key, optionPresent(opt) === true)
      }
    }
  }
  return { held: [...seen.values()].filter(Boolean).length, known: seen.size }
})

// Progress over all of a port's requirements, not only the required ones. The
// game page's badge answers "can I play this"; this answers "how much of what
// this port can use do I have", which is the ROM library's question. Each row
// still carries its own required / adds content flag, so the two never look
// contradictory.
function portProgress(port) {
  const reqs = port.romDependencies ?? []
  return { met: reqs.filter(requirementMet).length, total: reqs.length }
}

// Files are matched by checksum, so which port's button was pressed only
// decides where an unmatched file is reported — anything recognised is filed
// correctly regardless.
async function addFiles(port) {
  addError.value = null
  addingFor.value = port._itemTitle
  try {
    const paths = await SelectROMFiles()
    if (!paths?.length) return
    const matched = await AddROMFiles(port._itemTitle, paths, false)
    if (!matched?.length) {
      addError.value = `None of the selected files matched a ROM ${port.title || port._itemTitle} can use.`
    }
    await load()
  } catch (e) {
    addError.value = String(e)
  } finally {
    addingFor.value = null
  }
}
</script>

<template>
  <div class="rom-library">
    <header class="head">
      <h1>ROMs</h1>
      <p class="sub">
        Every ROM the ports in your catalog can use. Files are identified by
        checksum, so a rename or a different extension makes no difference:
        drop them anywhere on this window to add them.
      </p>
      <p v-if="!loading && totals.known" class="tally mono">
        {{ totals.held }} of {{ totals.known }} known dumps in your library
      </p>
    </header>

    <p v-if="error" class="error-text">{{ error }}</p>
    <p v-if="addError" class="error-text">{{ addError }}</p>

    <p v-if="loading" class="empty">Loading&hellip;</p>
    <p v-else-if="!library.ports?.length" class="empty">
      No port in your catalog declares a ROM requirement.
    </p>

    <section v-for="port in library.ports" :key="port._itemTitle" class="port">
      <div class="port-head">
        <h2>{{ port.title || port._itemTitle }}</h2>
        <span class="progress mono">
          {{ portProgress(port).met }} of {{ portProgress(port).total }}
        </span>
        <button
          class="btn-outline add"
          :disabled="addingFor === port._itemTitle"
          @click="addFiles(port)"
        >{{ addingFor === port._itemTitle ? 'Adding&hellip;' : 'Add file&hellip;' }}</button>
      </div>

      <!-- The same list the game page shows, so a row means the same thing in
           both places. Rows start collapsed here: this view spans every port,
           and opening each unmet dump would bury the summary. -->
      <RomRequirements
        :requirements="port.romDependencies"
        :status="library.status"
      />
    </section>
  </div>
</template>

<style lang="scss" scoped>
.rom-library {
  padding: var(--pad-page);
  max-width: 760px;
}

.head h1 {
  margin: 0;
  font-size: 22px;
  font-weight: 600;
  letter-spacing: -0.015em;
  color: var(--text);
}

.sub {
  margin: 8px 0 0;
  max-width: 62ch;
  font-size: 13px;
  line-height: 1.5;
  color: var(--dim);
  text-wrap: pretty;
}

.tally {
  margin: 10px 0 0;
  font-size: 12px;
  color: var(--dim2);
}

.empty {
  margin-top: 28px;
  font-size: 13.5px;
  color: var(--dim2);
}

.error-text {
  margin-top: 20px;
  font-size: 12.5px;
  color: var(--bad);
}

.port { margin-top: 30px; }

.port-head {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;

  h2 {
    margin: 0;
    font-size: 14.5px;
    font-weight: 600;
    letter-spacing: -0.01em;
    color: var(--text);
  }
}

.progress {
  font-size: 11.5px;
  color: var(--dim2);
}

.add { margin-left: auto; }
</style>
