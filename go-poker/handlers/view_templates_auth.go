package handlers

import "fmt"

func renderBaseLayout(title, content, activeNav, currentUsername string, walletBalance int64, isAdmin bool) string {
	navHTML := ""
	if currentUsername != "" {
		brandHref := "/lobby"
		centerHTML := fmt.Sprintf(`
                <a href="/lobby" class="nav-item %s">Lobby</a>
                <a href="/wallet" class="nav-item %s">Wallet</a>`,
			activeClass(activeNav == "lobby"), activeClass(activeNav == "wallet"))
		walletBadgeHTML := fmt.Sprintf(`
                <a href="/wallet" class="wallet-badge" title="Wallet Balance">
                    <span class="wallet-icon">
                        <svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12V7a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-5"/><path d="M16 12h5v4h-5z"/></svg>
                    </span>
                    <span class="wallet-val" id="wallet-badge-value">%d ETB</span>
                </a>`, walletBalance)

		// Admins aren't players: no Lobby/Wallet, and the brand mark points
		// back at the dashboard instead of a lobby they can't use.
		if isAdmin {
			brandHref = "/admin"
			centerHTML = `<a href="/admin" class="nav-item active">Admin Dashboard</a>`
			walletBadgeHTML = ""
		}

		navHTML = fmt.Sprintf(`
        <nav class="app-nav">
            <div class="nav-brand">
                <a href="%s" class="brand-link">
                    <span class="brand-chip">
                        <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor"><path d="M12 2C9 7 4 9 4 14a6 6 0 0 0 10.5 4l-.5 4h2l-.5-4A6 6 0 0 0 20 14c0-5-5-7-8-12z"/></svg>
                    </span>
                    <span class="brand-text">GOLDEN <strong>POKER</strong></span>
                </a>
            </div>
            <div class="nav-center">%s</div>
            <div class="nav-user">
                %s
                <span class="user-badge" title="Logged in as %s">
                    <span class="user-avatar">%c</span>
                    <span class="user-name">%s</span>
                </span>
                <button hx-post="/api/auth/logout" class="btn-logout" title="Logout">
                    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>
                </button>
            </div>
        </nav>`,
			brandHref,
			centerHTML,
			walletBadgeHTML,
			currentUsername,
			usernameInitial(currentUsername),
			currentUsername,
		)
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">
    <meta name="theme-color" content="#07090e">
    <title>%s | Golden Poker</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@400;500;600;700;800&family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
    <link rel="stylesheet" href="/static/poker/css/poker_modern.css">
    <script src="https://unpkg.com/htmx.org@1.9.12"></script>
    <script src="https://unpkg.com/htmx.org@1.9.12/dist/ext/sse.js"></script>
</head>
<body class="poker-app">
    %s
    <main class="app-main">
        %s
    </main>
    <div id="toast-container"></div>
    <div id="confirm-modal-overlay" class="confirm-modal-overlay hidden">
        <div class="confirm-modal-card">
            <p class="confirm-modal-message" id="confirm-modal-message"></p>
            <div class="confirm-modal-actions">
                <button type="button" class="btn-secondary" id="confirm-modal-cancel">Cancel</button>
                <button type="button" class="btn-gold" id="confirm-modal-accept">Confirm</button>
            </div>
        </div>
    </div>
    <script src="/static/poker/js/poker_htmx.js"></script>
</body>
</html>`, title, navHTML, content)
}

func activeClass(isActive bool) string {
	if isActive {
		return "active"
	}
	return ""
}

func usernameInitial(name string) byte {
	if len(name) > 0 {
		return name[0]
	}
	return 'U'
}

func renderLoginHTML() string {
	return `
    <div class="auth-wrapper">
        <div class="auth-card">
            <div class="auth-header">
                <div class="auth-logo">
                    <span class="chip-glow">
                        <svg viewBox="0 0 24 24" width="36" height="36" fill="currentColor"><path d="M12 2C9 7 4 9 4 14a6 6 0 0 0 10.5 4l-.5 4h2l-.5-4A6 6 0 0 0 20 14c0-5-5-7-8-12z"/></svg>
                    </span>
                </div>
                <h1>Golden Poker</h1>
                <p>Sign in to enter live tables</p>
            </div>

            <form hx-post="/api/auth/login" hx-target="#auth-error" class="auth-form">
                <div id="auth-error"></div>
                <div class="form-group">
                    <label for="login">Username or Phone</label>
                    <input type="text" id="login" name="login" required placeholder="Username or phone (e.g. 0912345678)" autocomplete="username">
                </div>
                <div class="form-group">
                    <label for="password">Password</label>
                    <div class="password-field-wrapper">
                        <input type="password" id="password" name="password" required placeholder="••••••••" autocomplete="current-password">
                        <button type="button" class="btn-toggle-password" onclick="togglePasswordVisibility('password', this)" aria-label="Show password">Show</button>
                    </div>
                </div>
                <button type="submit" class="btn-primary btn-block">Sign In</button>
            </form>

            <div class="auth-footer">
                <p>New to Golden Poker? <a href="/register">Create Account</a></p>
            </div>
        </div>
    </div>`
}

func renderRegisterHTML() string {
	return `
    <div class="auth-wrapper">
        <div class="auth-card">
            <div class="auth-header">
                <div class="auth-logo">
                    <span class="chip-glow">
                        <svg viewBox="0 0 24 24" width="36" height="36" fill="currentColor"><path d="M12 2C9 7 4 9 4 14a6 6 0 0 0 10.5 4l-.5 4h2l-.5-4A6 6 0 0 0 20 14c0-5-5-7-8-12z"/></svg>
                    </span>
                </div>
                <h1>Create Account</h1>
                <p>Join the premier Texas Hold'em platform in Ethiopia</p>
            </div>

            <form hx-post="/api/auth/register" hx-target="#auth-error" class="auth-form">
                <div id="auth-error"></div>
                <div class="form-group">
                    <label for="username">Username</label>
                    <input type="text" id="username" name="username" required placeholder="Choose a player name" autocomplete="username">
                </div>
                <div class="form-group">
                    <label for="phone_number">Phone Number (Ethiopian: 09... or 07...)</label>
                    <input type="tel" id="phone_number" name="phone_number" required placeholder="0911223344 or +251911223344" autocomplete="tel">
                </div>
                <div class="form-group">
                    <label for="password">Password</label>
                    <div class="password-field-wrapper">
                        <input type="password" id="password" name="password" required placeholder="••••••••" autocomplete="new-password">
                        <button type="button" class="btn-toggle-password" onclick="togglePasswordVisibility('password', this)" aria-label="Show password">Show</button>
                    </div>
                </div>
                <div class="form-group">
                    <label for="referral_code">Referral Code (Optional)</label>
                    <input type="text" id="referral_code" name="referral_code" placeholder="8-character code">
                </div>
                <button type="submit" class="btn-primary btn-block">Create Account</button>
            </form>

            <div class="auth-footer">
                <p>Already have an account? <a href="/login">Sign in</a></p>
            </div>
        </div>
    </div>`
}
