/** Client-side reading of a pasted CBE transfer SMS.
 *
 *  This is a convenience, not a control: it tells someone their paste is
 *  missing the link before they wait on a request, and it shows them which
 *  link was found so a paste of the wrong message is obvious. The server
 *  re-extracts from the same text and its answer is the one that counts —
 *  see utilities/receipt.go, which this mirrors.
 */

/** Matches the receipt link CBE puts at the end of a transfer SMS. The host is
 *  pinned to cbe.com.et so the feedback link in the same message
 *  (forms.gle/...) is never picked up instead. */
const CBE_RECEIPT_URL = /\bhttps?:\/\/[a-z0-9.-]*cbe\.com\.et\/[^\s<>"']+/i

/** Punctuation a sentence leaves stuck to the end of a URL. */
const TRAILING_PUNCTUATION = /[.,;:!?)\]}'"]+$/

/** The receipt_url column's width. */
const MAX_URL_LENGTH = 255

export type ReceiptParseResult =
  | { ok: true; url: string }
  | { ok: false; reason: string }

export function extractCbeReceiptUrl(pasted: string): ReceiptParseResult {
  const match = CBE_RECEIPT_URL.exec(pasted.trim())
  if (!match) {
    return {
      ok: false,
      reason: 'No CBE receipt link found. Paste the whole SMS, including the https://... link at the end.',
    }
  }

  const trimmed = match[0].replace(TRAILING_PUNCTUATION, '')

  let parsed: URL
  try {
    parsed = new URL(trimmed)
  } catch {
    return { ok: false, reason: "That link doesn't look like a CBE receipt link." }
  }
  if (parsed.pathname.replace(/\//g, '') === '') {
    return { ok: false, reason: "That link has no receipt in it — check you copied the whole message." }
  }

  // Scheme and host are case-insensitive and safe to normalize; the path
  // holds the receipt token, where case matters.
  parsed.protocol = 'https:'
  parsed.search = ''
  parsed.hash = ''

  const url = parsed.toString()
  if (url.length > MAX_URL_LENGTH) {
    return { ok: false, reason: 'That receipt link is too long to be a CBE link.' }
  }
  return { ok: true, url }
}
