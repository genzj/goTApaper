// The API token, and how the interface comes to hold it.
//
// The interface authenticates like any other client: a bearer token on every
// API request. It is kept in local storage so it is asked for once rather than
// on every visit. Local storage is scoped to the origin, port included, which
// is why the daemon tries to keep the same port across restarts.
//
// There are two ways in:
//   * the tray opens the page with a single-use ticket in the URL fragment,
//     which is traded for the token here and then removed from the address
//     bar -- no typing;
//   * otherwise the page asks, and the user pastes the output of
//     "goTApaper token".

import { writable } from 'svelte/store'

const STORAGE_KEY = 'gotapaper.api-token'

function read(): string {
  try {
    return localStorage.getItem(STORAGE_KEY) ?? ''
  } catch {
    // Storage can be disabled outright; the user then signs in each visit.
    return ''
  }
}

/** The token, or "" when signed out. */
export const token = writable(read())

token.subscribe((value) => {
  try {
    if (value) localStorage.setItem(STORAGE_KEY, value)
    else localStorage.removeItem(STORAGE_KEY)
  } catch {
    // See read.
  }
})

/** signOut forgets the token, for a rejected one or on request. */
export function signOut(): void {
  token.set('')
}

/**
 * verify checks a candidate token against the API before keeping it, so a
 * typo is reported at the sign-in prompt rather than as a page of errors.
 */
export async function verify(candidate: string): Promise<boolean> {
  const resp = await fetch('/api/v1/status', {
    headers: { Authorization: `Bearer ${candidate}` },
  })
  return resp.ok
}

/**
 * redeemTicket trades a ticket from the URL fragment for the token, if there
 * is one, and removes it from the address bar either way.
 *
 * It is removed before the exchange, not after: the ticket is single-use, so
 * once this runs it is spent whatever the outcome, and leaving it in place
 * would only invite a reload that fails.
 */
export async function redeemTicket(): Promise<string | null> {
  const params = new URLSearchParams(window.location.hash.replace(/^#/, ''))
  const ticket = params.get('ticket')
  if (!ticket) return null

  params.delete('ticket')
  const rest = params.toString()
  history.replaceState(
    null,
    '',
    window.location.pathname + window.location.search + (rest ? `#${rest}` : ''),
  )

  const resp = await fetch('/api/v1/auth/ticket', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ticket }),
  })
  if (!resp.ok) {
    let message = 'This link has expired. Open goTApaper from the tray menu again.'
    try {
      const body = await resp.json()
      if (body?.error) message = body.error
    } catch {
      // Keep the default.
    }
    throw new Error(message)
  }

  const body = (await resp.json()) as { token?: string }
  if (!body.token) throw new Error('The daemon did not return a token.')
  token.set(body.token)
  return body.token
}
