import { ref } from 'vue'

// keys from the search box in the header, handled by the catalog list
export type SearchKey = 'up' | 'down' | 'enter' | 'shift-enter'

// DOM id of the highlighted row, for aria-activedescendant
export const activeOption = ref('')

// text is what the box holds, the list may still show results for an older query
type Handler = (k: SearchKey, text: string) => boolean
let handler: Handler | null = null

export function setSearchKeyHandler(fn: Handler | null) {
  handler = fn
}

// true when the key was handled and the caller should preventDefault
export function sendSearchKey(k: SearchKey, text = ''): boolean {
  return handler ? handler(k, text) : false
}
