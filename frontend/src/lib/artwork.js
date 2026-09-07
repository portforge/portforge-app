// Catalog artwork is stored at print resolution — covers are 600x900, key art is
// 3840x1240 — while the UI shows them at a fraction of that. Handing the WebView
// the full-size file makes it hold a bitmap up to 18MB and resample it down on
// every paint, which is what made hover states lag on the Library grid.
//
// Every artwork URL therefore carries the width it will actually be drawn at, and
// the Go asset handler serves a cached downscale. See thumbs.go.

// Past 2x the extra pixels cost more to decode than they add on screen.
const scale = () => Math.min(window.devicePixelRatio || 1, 2)

// item is the MediaItem the artwork belongs to; its own _itemType names the
// directory it lives in. Taking the type from the item rather than assuming one
// is the point: the ItemType folders have been renamed twice, and a hardcoded
// name here broke every image in the app without breaking anything else, which
// made it look like an artwork bug rather than a rename that missed a file.
//
// cssWidth is the width the image occupies in CSS pixels. Omit it to get the
// original file, which is what the lightbox wants.
export function artworkUrl(item, fileName, cssWidth) {
  const type = item?._itemType
  const title = item?._itemTitle
  if (!type || !title || !fileName) return ''
  const url = `/mediaitems/${encodeURIComponent(type)}/${encodeURIComponent(title)}/.artwork/${encodeURIComponent(fileName)}`
  if (!cssWidth) return url
  return `${url}?w=${Math.round(cssWidth * scale())}`
}

// The widths the UI draws artwork at, kept here so they stay in step with the
// stylesheets that set them.
export const ART_WIDTH = {
  gridCover: 200, // Library grid cell at the default window width
  heroCover: 150, // .hero-cover on the game page
  overlayCover: 200, // the Now Playing card
  screenshot: 320, // one cell of the three-column .shots grid
  // The key art band is full-bleed, so its width follows the window.
  hero: () => Math.max(960, Math.round(window.innerWidth)),
  // The blurred stand-in for missing key art is scaled up and blurred by 28px,
  // so detail in the source is thrown away regardless — a small one is enough.
  heroFallback: 384,
}
