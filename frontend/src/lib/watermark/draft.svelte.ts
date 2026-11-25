// The editor's working copy of the layers, with undo and redo.
//
// Edits mutate `layers` directly, which is what Svelte's bindings want. A
// history entry is a snapshot taken *before* a gesture: commit() records one,
// and a run of edits within one gesture -- dragging a slider, moving a layer --
// is coalesced by passing the same key, so one undo reverts the whole drag.

import type { WatermarkLayer } from '../api'

const HISTORY_LIMIT = 100
/** Edits with the same key closer together than this are one gesture. */
const COALESCE_MS = 800

function snapshot(layers: WatermarkLayer[]): string {
  return JSON.stringify(layers)
}

export class Draft {
  layers = $state<WatermarkLayer[]>([])
  selected = $state(0)
  /** The last saved list, to tell whether there is anything to save. */
  saved = $state('[]')

  #undo: string[] = []
  #redo: string[] = []
  #lastKey = ''
  #lastAt = 0
  canUndo = $state(false)
  canRedo = $state(false)

  readonly dirty = $derived(snapshot(this.layers) !== this.saved)

  /** Replaces everything, as after loading or saving; clears the history. */
  reset(layers: WatermarkLayer[]): void {
    this.layers = JSON.parse(snapshot(layers))
    this.saved = snapshot(this.layers)
    this.selected = Math.min(this.selected, Math.max(0, this.layers.length - 1))
    this.#undo = []
    this.#redo = []
    this.#sync()
  }

  markSaved(layers: WatermarkLayer[]): void {
    // Saved layers come back with ids; keep the history, adopt the ids.
    this.layers = JSON.parse(snapshot(layers))
    this.saved = snapshot(this.layers)
  }

  /**
   * commit records the state before an edit. Call it, then mutate. With a
   * key, a quick run of edits under the same key is one undo step.
   */
  commit(key = ''): void {
    const now = Date.now()
    if (key && key === this.#lastKey && now - this.#lastAt < COALESCE_MS) {
      this.#lastAt = now
      return
    }
    this.#lastKey = key
    this.#lastAt = now
    this.#undo.push(snapshot(this.layers))
    if (this.#undo.length > HISTORY_LIMIT) this.#undo.shift()
    this.#redo = []
    this.#sync()
  }

  undo(): void {
    const prev = this.#undo.pop()
    if (prev === undefined) return
    this.#redo.push(snapshot(this.layers))
    this.layers = JSON.parse(prev)
    this.#lastKey = ''
    this.#clampSelection()
    this.#sync()
  }

  redo(): void {
    const next = this.#redo.pop()
    if (next === undefined) return
    this.#undo.push(snapshot(this.layers))
    this.layers = JSON.parse(next)
    this.#lastKey = ''
    this.#clampSelection()
    this.#sync()
  }

  /** The list as the server takes it: sort order follows position. */
  forSave(): WatermarkLayer[] {
    return (JSON.parse(snapshot(this.layers)) as WatermarkLayer[]).map((l, i) => ({
      ...l,
      sortOrder: i,
    }))
  }

  #clampSelection(): void {
    this.selected = Math.min(this.selected, Math.max(0, this.layers.length - 1))
  }

  #sync(): void {
    this.canUndo = this.#undo.length > 0
    this.canRedo = this.#redo.length > 0
  }
}
