// Pure helpers for the watermark editor: layer defaults, anchor geometry and
// colours. Kept free of Svelte so the rules are in one readable place.

import type { FractionRect, WatermarkLayer } from '../api'

export const ANCHORS = [
  'top-left',
  'top-center',
  'top-right',
  'middle-left',
  'middle-center',
  'middle-right',
  'bottom-left',
  'bottom-center',
  'bottom-right',
] as const

export const ALIGNMENTS = ['left', 'center', 'right'] as const

/** How far into the text box, as fractions of its size, each anchor sits. */
export function anchorFactors(anchor: string): [number, number] {
  const [v, h] = anchor.split('-')
  const fx = h === 'left' ? 0 : h === 'right' ? 1 : 0.5
  const fy = v === 'top' ? 0 : v === 'bottom' ? 1 : 0.5
  return [fx, fy]
}

/** A fresh layer, matching the server's defaults (store.NewWatermarkLayer). */
export function newLayer(font: string): WatermarkLayer {
  return {
    id: 0,
    sortOrder: 0,
    enabled: true,
    name: '',
    template: '{{.Title}}',
    font,
    size: 1.6,
    color: 'ffffff',
    anchor: 'bottom-right',
    x: 0.97,
    y: 0.95,
    alignment: 'right',
    linespace: 1.4,
    background: { color: '', hThroughout: false, vThroughout: false, padding: [0, 0, 0, 0] },
    shadow: { color: '00000088', dx: 0.06, dy: 0.06 },
    outline: { color: '', width: 0 },
  }
}

/**
 * cloneLayer deep-copies a layer. JSON rather than structuredClone, which
 * refuses Svelte's state proxies; a layer holds only plain data.
 */
export function cloneLayer(l: WatermarkLayer): WatermarkLayer {
  return JSON.parse(JSON.stringify(l)) as WatermarkLayer
}

export function layerLabel(l: WatermarkLayer, index: number): string {
  return l.name.trim() || `Layer ${index + 1}`
}

/**
 * placeFromBox works out where a dragged text box now is: the anchor nearest
 * its centre, and that anchor's position. The same rule on every anchor and
 * axis, because x and y are simply where the anchor point sits.
 */
export function placeFromBox(box: FractionRect): { anchor: string; x: number; y: number } {
  const cx = box.x + box.w / 2
  const cy = box.y + box.h / 2
  const col = cx < 1 / 3 ? 'left' : cx > 2 / 3 ? 'right' : 'center'
  const row = cy < 1 / 3 ? 'top' : cy > 2 / 3 ? 'bottom' : 'middle'
  const anchor = `${row}-${col}`
  const [fx, fy] = anchorFactors(anchor)
  return { anchor, x: round4(box.x + fx * box.w), y: round4(box.y + fy * box.h) }
}

/**
 * reanchor changes a layer's anchor without moving its text: x and y move to
 * where the new anchor point of the same box is.
 */
export function reanchor(l: WatermarkLayer, anchor: string, box?: FractionRect): void {
  if (box && box.w > 0) {
    const [fx, fy] = anchorFactors(anchor)
    l.x = round4(box.x + fx * box.w)
    l.y = round4(box.y + fy * box.h)
  }
  l.anchor = anchor
}

/**
 * Margins are derived, not stored: the distance from the anchor's edge, which
 * is what "5 % from the right" means. Centre axes have none.
 */
export function marginOf(l: WatermarkLayer, axis: 'x' | 'y'): number | null {
  const [fx, fy] = anchorFactors(l.anchor)
  const f = axis === 'x' ? fx : fy
  const v = axis === 'x' ? l.x : l.y
  if (f === 0) return v
  if (f === 1) return 1 - v
  return null
}

export function setMargin(l: WatermarkLayer, axis: 'x' | 'y', margin: number): void {
  const [fx, fy] = anchorFactors(l.anchor)
  const f = axis === 'x' ? fx : fy
  const v = f === 1 ? 1 - margin : margin
  if (axis === 'x') l.x = round4(v)
  else l.y = round4(v)
}

export function round4(v: number): number {
  return Math.round(v * 10000) / 10000
}

export function clamp(v: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, v))
}

/** Splits rrggbb[aa] into the #rrggbb a colour input takes and an alpha 0–1. */
export function splitColor(hex: string): { rgb: string; alpha: number } {
  let h = (hex || '').replace(/^#/, '')
  if (h.length === 3) h = h.split('').map((c) => c + c).join('')
  if (!/^[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$/.test(h)) return { rgb: '#000000', alpha: 1 }
  const alpha = h.length === 8 ? parseInt(h.slice(6), 16) / 255 : 1
  return { rgb: `#${h.slice(0, 6).toLowerCase()}`, alpha }
}

export function joinColor(rgb: string, alpha: number): string {
  const base = rgb.replace(/^#/, '').toLowerCase()
  if (alpha >= 0.999) return base
  const a = Math.round(clamp(alpha, 0, 1) * 255)
  return base + a.toString(16).padStart(2, '0')
}

export function validColor(hex: string): boolean {
  return /^#?([0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/.test(hex)
}

/** Aspect-ratio safe areas offered as guides: what each display shape keeps. */
export const ASPECT_GUIDES = [
  { label: '16:9', ratio: 16 / 9 },
  { label: '16:10', ratio: 16 / 10 },
  { label: '21:9', ratio: 21 / 9 },
  { label: '4:3', ratio: 4 / 3 },
] as const

/**
 * guideRect is the part of a picture with the given aspect ratio that a
 * display of another ratio shows when the wallpaper fills it, centred.
 */
export function guideRect(pictureRatio: number, displayRatio: number): FractionRect {
  if (displayRatio > pictureRatio) {
    const h = pictureRatio / displayRatio
    return { x: 0, y: (1 - h) / 2, w: 1, h }
  }
  const w = displayRatio / pictureRatio
  return { x: (1 - w) / 2, y: 0, w, h: 1 }
}
