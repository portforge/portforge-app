/**
 * Artwork type definitions and per-MediaItem-type defaults.
 *
 * RATIOS maps a lowercase artworkType to its CSS aspect-ratio value.
 * Cover is the universal fallback for unknown item types and artwork types.
 */

const RATIOS = {
  'cover':       '2 / 3',
  'n64boxfront': '1.37 / 1',
  'nesboxfront': '3 / 4',
  'xbox360boxfront': '0.71 / 1',
  'ps1boxfront': '1 / 1',
}

/** Default artworkType per MediaItem _itemType (lowercase keys). */
const DEFAULT_BY_ITEM_TYPE = {
  'n64rom':  'N64BoxFront',
  'nesrom':  'NESBoxFront',
  'xbox360rom':  'Xbox360BoxFront',
  'ps1rom':  'PS1BoxFront',
}

/** CSS aspect-ratio string for a given artworkType name. Falls back to Cover ratio. */
export function artworkAspectRatio(artworkType) {
  return RATIOS[artworkType?.toLowerCase()] ?? RATIOS['cover']
}

/** Default artworkType for a MediaItem based on its _itemType field. */
export function defaultArtworkType(itemType) {
  return DEFAULT_BY_ITEM_TYPE[itemType?.toLowerCase()] ?? 'Cover'
}

/**
 * Returns the primary Artwork entry from an array. Prefers the entry whose
 * artworkType matches the item's default type; otherwise takes the first entry.
 */
export function primaryArtwork(artworkArray, itemType) {
  if (!artworkArray?.length) return null
  const preferred = defaultArtworkType(itemType)
  return artworkArray.find(a => a.artworkType?.toLowerCase() === preferred.toLowerCase())
    ?? artworkArray[0]
}
