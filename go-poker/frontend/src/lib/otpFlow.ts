/** Flow state that has to survive a page refresh.
 *
 *  Mobile browsers on a flaky connection reload pages more than you would
 *  like, and both OTP flows span two screens. Keeping the phone (and, for a
 *  reset, the token that authorizes the password change) in sessionStorage
 *  means a refresh lands the user back where they were instead of costing them
 *  an SMS from a budget of five per hour. It is tab-scoped and cleared when the
 *  tab closes.
 */

const PHONE_KEY = 'poker.otp.phone'
const TOKEN_KEY = 'poker.reset.token'

function read(key: string): string {
  try {
    return sessionStorage.getItem(key) ?? ''
  } catch {
    // Private-mode Safari and some embedded webviews throw on access; the flow
    // still works within a single unbroken screen sequence.
    return ''
  }
}

function write(key: string, value: string) {
  try {
    if (value) sessionStorage.setItem(key, value)
    else sessionStorage.removeItem(key)
  } catch {
    /* see read() */
  }
}

export const otpFlow = {
  phone: () => read(PHONE_KEY),
  setPhone: (phone: string) => write(PHONE_KEY, phone),
  resetToken: () => read(TOKEN_KEY),
  setResetToken: (token: string) => write(TOKEN_KEY, token),
  clear: () => {
    write(PHONE_KEY, '')
    write(TOKEN_KEY, '')
  },
}
