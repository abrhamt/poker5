package handlers

import "fmt"

// renderBaseLayout wraps the admin dashboard's markup in the site chrome.
//
// This is the last server-rendered HTML in the app; everything player-facing is
// the React bundle. Its stylesheet and script ship inside that bundle (see
// frontend/public/legacy/) and are served by the web server out front, so this
// server still hands out no files of its own.
func renderBaseLayout(title, content, currentUsername string) string {
	navHTML := ""
	if currentUsername != "" {
		navHTML = fmt.Sprintf(`
        <nav class="app-nav">
            <div class="nav-brand">
                <a href="/admin" class="brand-link">
                    <span class="brand-chip">
                        <svg viewBox="0 0 24 24" width="18" height="18" fill="currentColor"><path d="M12 2C9 7 4 9 4 14a6 6 0 0 0 10.5 4l-.5 4h2l-.5-4A6 6 0 0 0 20 14c0-5-5-7-8-12z"/></svg>
                    </span>
                    <span class="brand-text">GOLDEN <strong>POKER</strong></span>
                </a>
            </div>
            <div class="nav-center"><a href="/admin" class="nav-item active">Admin Dashboard</a></div>
            <div class="nav-user">
                <span class="user-badge" title="Logged in as %s">
                    <span class="user-avatar">%c</span>
                    <span class="user-name">%s</span>
                </span>
                <button hx-post="/api/auth/logout" class="btn-logout" title="Logout">
                    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>
                </button>
            </div>
        </nav>`,
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
    <link rel="stylesheet" href="/legacy/admin.css">
    <script src="https://unpkg.com/htmx.org@1.9.12"></script>
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
    <script src="/legacy/admin.js"></script>
</body>
</html>`, title, navHTML, content)
}

func usernameInitial(name string) byte {
	if len(name) > 0 {
		return name[0]
	}
	return 'U'
}
