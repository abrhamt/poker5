package handlers

import (
	"fmt"
	"strings"
	"time"

	poker "github.com/zuse/poker5/go-poker"
)

// remainingSecondsFromMs is the initial (pre-JS-tick) display value for a
// countdown whose authoritative end time is an absolute Unix-ms timestamp.
func remainingSecondsFromMs(endsAtMs int64) int {
	if endsAtMs == 0 {
		return 0
	}
	remaining := endsAtMs - time.Now().UnixMilli()
	if remaining < 0 {
		return 0
	}
	return int((remaining + 999) / 1000)
}

func renderTablePageHTML(roomCode, currentUsername string, walletBalance int64, isAdmin bool, state map[string]interface{}) string {
	content := fmt.Sprintf(`
    <div class="table-screen" hx-ext="sse" sse-connect="/api/events?table_id=%s">
        <div class="table-top-bar">
            <div class="table-info">
                <a href="/lobby" class="btn-back" title="Back to Lobby">Lobby</a>
                <div class="room-pill">
                    <span class="room-type-badge">Code:</span>
                    <span class="room-code-tag" id="room-code-display">%s</span>
                    <button type="button" class="btn-copy-code" onclick="copyRoomCode('%s')" title="Copy 5-digit Room Code">
                        <svg viewBox="0 0 24 24" width="13" height="13" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/></svg>
                        <span id="copy-btn-text">Copy</span>
                    </button>
                </div>
            </div>
            <div class="table-controls">
                <button hx-post="/api/table/%s/leave" hx-confirm="Are you sure you want to leave the table and cash out your chips?" data-confirm-danger="true" class="btn-leave" title="Leave & Cash Out">Leave Table</button>
            </div>
        </div>


        <div id="table-view-container" hx-trigger="sse:game-state, load" hx-get="/table/%s" hx-target="#table-view-container">
            %s
        </div>
    </div>`, roomCode, roomCode, roomCode, roomCode, roomCode, renderTablePartialHTML(roomCode, currentUsername, walletBalance, state))

	return renderBaseLayout("Table #"+roomCode, content, "table", currentUsername, walletBalance, isAdmin)
}

func renderTablePartialHTML(roomCode, currentUsername string, walletBalance int64, state map[string]interface{}) string {
	pot, _ := state["pot"].(int)
	phase, _ := state["phase"].(string)
	currentBet, _ := state["current_bet"].(int)
	communityCards, _ := state["community_cards"].([]string)
	playersList, _ := state["players"].([]map[string]interface{})
	notifications, _ := state["notifications"].([]string)
	gameStarted, _ := state["game_started"].(bool)
	intermission, _ := state["intermission"].(bool)
	turnRemaining, _ := state["turn_remaining_seconds"].(int)
	turnTotal, _ := state["turn_total_seconds"].(int)
	currentTurnPlayer, _ := state["current_turn_player"].(string)
	isMyTurn, _ := state["is_my_turn"].(bool)
	maxPlayers, _ := state["max_players"].(int32)
	if maxPlayers <= 0 {
		maxPlayers = 6
	}
	roomType, _ := state["room_type"].(string)
	hostUserID, _ := state["host_user_id"].(int64)
	countdownActive, _ := state["countdown_active"].(bool)
	countdownRemaining, _ := state["countdown_remaining_seconds"].(int)
	countdownEndsAtMs, _ := state["countdown_ends_at_ms"].(int64)
	buyIn, _ := state["buy_in"].(int64)

	if turnTotal <= 0 {
		turnTotal = 25
	}

	timerPct := 0
	if turnTotal > 0 {
		timerPct = (turnRemaining * 100) / turnTotal
	}

	var mePlayer map[string]interface{}
	for _, p := range playersList {
		pName, _ := p["name"].(string)
		if pName == currentUsername {
			mePlayer = p
			break
		}
	}

	winningCards := map[string]bool{}
	if intermission {
		_, _, _, cards := extractWinnerInfo(state)
		for _, c := range cards {
			winningCards[c] = true
		}
	}

	seatsHTML := ""
	for i := 0; i < int(maxPlayers); i++ {
		seatClass := fmt.Sprintf("seat-pos-%d", i+1)
		if i < len(playersList) {
			p := playersList[i]
			pName, _ := p["name"].(string)
			chips, _ := p["chips"].(int)
			roundBet, _ := p["round_bet"].(int)
			folded, _ := p["folded"].(bool)
			allIn, _ := p["all_in"].(bool)
			isDealer, _ := p["dealer"].(bool)
			isSB, _ := p["small_blind"].(bool)
			isBB, _ := p["big_blind"].(bool)
			isTurn := (pName == currentTurnPlayer && gameStarted && !intermission)

			statusClass := ""
			if folded {
				statusClass = "player-folded"
			} else if allIn {
				statusClass = "player-allin"
			} else if isTurn {
				statusClass = "player-turn-active"
			}

			dealerBadge := ""
			if isDealer {
				dealerBadge = `<span class="badge-dealer" title="Dealer">D</span>`
			} else if isSB {
				dealerBadge = `<span class="badge-sb" title="Small Blind">SB</span>`
			} else if isBB {
				dealerBadge = `<span class="badge-bb" title="Big Blind">BB</span>`
			}

			cardsHTML := renderSeatCards(p, intermission, winningCards)

			betBadge := ""
			if roundBet > 0 {
				betBadge = fmt.Sprintf(`<div class="seat-bet"><span class="chip-dot"></span> %d</div>`, roundBet)
			}

			seatsHTML += fmt.Sprintf(`
            <div class="seat-wrapper %s %s">
                <div class="player-avatar">
                    <span class="avatar-letter">%c</span>
                    %s
                </div>
                <div class="player-card-box">
                    %s
                </div>
                <div class="player-meta">
                    <span class="player-name">%s</span>
                    <span class="player-chips">%d chips</span>
                </div>
                %s
            </div>`, seatClass, statusClass, usernameInitial(pName), dealerBadge, cardsHTML, pName, chips, betBadge)
		} else {
			seatsHTML += fmt.Sprintf(`
            <div class="seat-wrapper %s seat-empty">
                <div class="empty-seat-slot">
                    <span class="plus-icon">+</span>
                    <span class="empty-text">Open</span>
                </div>
            </div>`, seatClass)
		}
	}

	communityHTML := ""
	for i := 0; i < 5; i++ {
		if i < len(communityCards) {
			card := communityCards[i]
			slotClass := "card-slot card-dealt"
			if winningCards[card] {
				slotClass += " card-winning"
			}
			communityHTML += fmt.Sprintf(`<div class="%s"><img src="/static/poker/cards/%s.svg" alt="%s"></div>`, slotClass, card, card)
		} else {
			communityHTML += `<div class="card-slot card-placeholder"></div>`
		}
	}

	notifHTML := ""
	if len(notifications) > 0 {
		lastIdx := len(notifications) - 1
		notifHTML = fmt.Sprintf(`<div class="table-log-banner"><span>%s</span></div>`, notifications[lastIdx])
	}

	isHost := hostUserID != 0 && state["is_host"] == true
	waitingHTML := renderWaitingBanner(roomType, isHost, roomCode, gameStarted, intermission, len(playersList), countdownActive, countdownRemaining, countdownEndsAtMs)

	dockHTML := renderActionDock(roomCode, currentUsername, mePlayer, isMyTurn, currentBet, pot, gameStarted, intermission, len(playersList), timerPct, turnRemaining, buyIn, roomType)

	showdownHTML := ""
	if intermission {
		showdownHTML = renderShowdownOverlay(state)
	}

	return fmt.Sprintf(`
    <div class="poker-table-felt">
        <div class="felt-border">
            <div class="felt-surface">
                <div class="pot-center">
                    <div class="pot-chip-stack">
                        <span class="pot-chip-icon">
                            <svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor"><circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="2" fill="none"/><circle cx="12" cy="12" r="6" fill="currentColor"/></svg>
                        </span>
                        <span class="pot-amount">%d</span>
                    </div>
                    <span class="phase-pill">%s</span>
                </div>

                <div class="community-cards-container">
                    %s
                </div>

                %s
            </div>
        </div>

        %s
        %s
    </div>

    %s
    %s
    <span id="wallet-badge-value" class="wallet-val" hx-swap-oob="true">%d ETB</span>`, pot, strings.ToUpper(phase), communityHTML, seatsHTML, notifHTML, waitingHTML, dockHTML, showdownHTML, walletBalance)
}

func extractCards(val interface{}) (string, string) {
	if val == nil {
		return "", ""
	}
	switch c := val.(type) {
	case [2]string:
		return c[0], c[1]
	case []string:
		if len(c) >= 2 {
			return c[0], c[1]
		}
	case []interface{}:
		if len(c) >= 2 {
			s1, _ := c[0].(string)
			s2, _ := c[1].(string)
			return s1, s2
		}
	}
	return "", ""
}

func renderSeatCards(p map[string]interface{}, intermission bool, winningCards map[string]bool) string {
	folded, _ := p["folded"].(bool)
	if folded {
		return `<div class="mini-cards folded-dim"><span class="folded-tag">FOLDED</span></div>`
	}

	isMe, _ := p["is_me"].(bool)
	if isMe || intermission {
		c1, c2 := extractCards(p["cards"])
		if c1 != "" && c1 != "1B" && c2 != "" && c2 != "1B" {
			c1Class, c2Class := "mini-card", "mini-card"
			if winningCards[c1] {
				c1Class += " card-winning"
			}
			if winningCards[c2] {
				c2Class += " card-winning"
			}
			return fmt.Sprintf(`
            <div class="mini-cards">
                <img class="%s" src="/static/poker/cards/%s.svg">
                <img class="%s" src="/static/poker/cards/%s.svg">
            </div>`, c1Class, c1, c2Class, c2)
		}
	}

	return `
    <div class="mini-cards">
        <img class="mini-card back" src="/static/poker/cards/1B.svg">
        <img class="mini-card back" src="/static/poker/cards/1B.svg">
    </div>`
}

// renderWaitingBanner shows the pre-hand countdown to everyone at the table,
// and (for private rooms only) a host-only "Start Now" button that skips
// the rest of the countdown. Public tables never get a manual start button
// — the countdown is the only trigger there.
func renderWaitingBanner(roomType string, isHost bool, roomCode string, gameStarted, intermission bool, playerCount int, countdownActive bool, countdownRemaining int, countdownEndsAtMs int64) string {
	if gameStarted || intermission || playerCount < 2 {
		return ""
	}

	startNowHTML := ""
	if roomType == "private" && isHost {
		startNowHTML = fmt.Sprintf(`<button hx-post="/api/table/%s/start" class="btn-gold btn-sm">Start Now</button>`, roomCode)
	}

	if !countdownActive {
		return fmt.Sprintf(`
    <div class="waiting-countdown-banner">
        <span class="countdown-text">Waiting for the table to fill...</span>
        %s
    </div>`, startNowHTML)
	}

	return fmt.Sprintf(`
    <div class="waiting-countdown-banner" data-countdown-until="%d">
        <span class="countdown-text">Hand starts in <span data-countdown-value>%d</span>s</span>
        %s
    </div>`, countdownEndsAtMs, countdownRemaining, startNowHTML)
}

func renderActionDock(roomCode, currentUsername string, me map[string]interface{}, isMyTurn bool, currentBet, pot int, gameStarted, intermission bool, playerCount, timerPct, turnRemaining int, buyIn int64, roomType string) string {
	if me == nil {
		if gameStarted && !intermission {
			return fmt.Sprintf(`
        <div class="action-dock spectator-dock">
            <div class="dock-spectator-msg">
                <p>Spectating Table #%s</p>
                <p class="dock-spectator-hint">A hand is in progress — you'll be able to sit down once it finishes.</p>
            </div>
        </div>`, roomCode)
		}
		sitLabel := fmt.Sprintf("Sit Down (%d ETB Buy-in)", buyIn)
		if roomType == "private" {
			sitLabel = "Sit Down (Bring Your Wallet Balance)"
		}
		return fmt.Sprintf(`
        <div class="action-dock spectator-dock">
            <div class="dock-spectator-msg">
                <p>Spectating Table #%s</p>
                <button hx-post="/api/table/%s/join" class="btn-gold btn-lg">%s</button>
            </div>
        </div>`, roomCode, roomCode, sitLabel)
	}

	c1, c2 := extractCards(me["cards"])
	handName, _ := me["hand_name"].(string)
	roundBet, _ := me["round_bet"].(int)
	chips, _ := me["chips"].(int)
	folded, _ := me["folded"].(bool)

	myCardsHTML := ""
	if c1 != "" && c1 != "1B" && c2 != "" && c2 != "1B" {
		myCardsHTML = fmt.Sprintf(`
        <div class="my-cards-display">
            <img class="hero-card" src="/static/poker/cards/%s.svg">
            <img class="hero-card" src="/static/poker/cards/%s.svg">
        </div>`, c1, c2)
	} else {
		myCardsHTML = `
        <div class="my-cards-display">
            <img class="hero-card back" src="/static/poker/cards/1B.svg">
            <img class="hero-card back" src="/static/poker/cards/1B.svg">
        </div>`
	}

	handBadge := ""
	if handName != "" && !folded {
		handBadge = fmt.Sprintf(`<div class="hand-eval-badge">%s</div>`, handName)
	}

	toCall := currentBet - roundBet
	if toCall < 0 {
		toCall = 0
	}

	checkCallText := "Check"
	checkCallAction := "check"
	if toCall > 0 {
		checkCallText = fmt.Sprintf("Call %d", toCall)
		checkCallAction = "call"
	}

	minRaise := currentBet + 20
	if minRaise > chips {
		minRaise = chips
	}

	timerBarHTML := ""
	if isMyTurn && !intermission {
		timerBarHTML = fmt.Sprintf(`
        <div class="turn-timer-container">
            <div class="turn-timer-bar" style="width: %d%%;"></div>
            <span class="timer-sec">%ds</span>
        </div>`, timerPct, turnRemaining)
	}

	controlsHTML := ""
	if !gameStarted && playerCount >= 2 {
		controlsHTML = `<div class="status-msg-box"><span class="msg-waiting">Waiting for the countdown to finish...</span></div>`
	} else if isMyTurn && !folded && !intermission {
		controlsHTML = fmt.Sprintf(`
        <div class="active-betting-controls">
            <div class="quick-bet-row">
                <button type="button" class="btn-chip" onclick="setRaiseAmount(%d)">Min</button>
                <button type="button" class="btn-chip" onclick="setRaiseAmount(%d)">1/2 Pot</button>
                <button type="button" class="btn-chip" onclick="setRaiseAmount(%d)">Pot</button>
                <button type="button" class="btn-chip" onclick="setRaiseAmount(%d)">All-In</button>
            </div>
            <div class="action-buttons-row">
                <button hx-post="/api/table/%s/act" hx-vals='{"action": "fold", "amount": 0}' class="btn-action btn-fold">Fold</button>
                <button hx-post="/api/table/%s/act" hx-vals='{"action": "%s", "amount": %d}' class="btn-action btn-call">%s</button>
                <div class="raise-input-group">
                    <input type="number" id="raise-amount-input" name="amount" value="%d" min="%d" max="%d" step="10" class="raise-field">
                    <button hx-post="/api/table/%s/act" hx-include="#raise-amount-input" hx-vals='{"action": "raise"}' class="btn-action btn-raise">Raise</button>
                </div>
            </div>
        </div>`,
			minRaise,
			pot/2,
			pot,
			chips,
			roomCode,
			roomCode, checkCallAction, toCall, checkCallText,
			minRaise, minRaise, chips,
			roomCode,
		)
	} else if folded {
		controlsHTML = `<div class="status-msg-box"><span class="msg-folded">Folded. Waiting for next deal...</span></div>`
	} else if !isMyTurn && gameStarted && !intermission {
		controlsHTML = `<div class="status-msg-box"><span class="msg-waiting">Waiting for opponent to act...</span></div>`
	}

	return fmt.Sprintf(`
    <div class="action-dock player-dock">
        %s
        <div class="dock-main">
            <div class="hero-hand-section">
                %s
                <div class="hero-stats">
                    <span class="hero-name">%s</span>
                    <span class="hero-stack">%d Chips</span>
                    %s
                </div>
            </div>
            <div class="hero-controls-section">
                %s
            </div>
        </div>
    </div>`, timerBarHTML, myCardsHTML, currentUsername, chips, handBadge, controlsHTML)
}

// extractWinnerInfo reads state["winner"] (a *poker.Winner in the normal
// direct-call path; the map[string]interface{} case is defensive in case
// state ever arrives pre-serialized). winningCards is nil if there is no
// winner recorded yet (e.g. mid-hand).
func extractWinnerInfo(state map[string]interface{}) (name string, amount int, handName string, winningCards []string) {
	winnerObj, exists := state["winner"]
	if !exists || winnerObj == nil {
		return "", 0, "", nil
	}
	switch w := winnerObj.(type) {
	case *poker.Winner:
		if w == nil {
			return "", 0, "", nil
		}
		return w.Name, w.Amount, w.HandName, w.WinningCards
	case poker.Winner:
		return w.Name, w.Amount, w.HandName, w.WinningCards
	case map[string]interface{}:
		n, _ := w["name"].(string)
		hn, _ := w["hand_name"].(string)
		amt := 0
		if a, ok := w["amount"].(int); ok {
			amt = a
		} else if af, ok := w["amount"].(float64); ok {
			amt = int(af)
		}
		var cards []string
		switch raw := w["winning_cards"].(type) {
		case []string:
			cards = raw
		case []interface{}:
			for _, v := range raw {
				if s, ok := v.(string); ok {
					cards = append(cards, s)
				}
			}
		}
		return n, amt, hn, cards
	}
	return "", 0, "", nil
}

func renderShowdownOverlay(state map[string]interface{}) string {
	winnerName := "Winner"
	amount := 0
	handName := "Winning Hand"

	name, amt, hn, _ := extractWinnerInfo(state)
	if name != "" {
		winnerName = name
	}
	amount = amt
	if hn != "" {
		handName = hn
	}

	intermissionEndsAtMs, _ := state["intermission_ends_at_ms"].(int64)

	return fmt.Sprintf(`
    <div class="showdown-overlay-modal">
        <div class="showdown-modal-card">
            <div class="showdown-trophy">
                <svg viewBox="0 0 24 24" width="48" height="48" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 9H4.5a2.5 2.5 0 0 1 0-5H6"/><path d="M18 9h1.5a2.5 2.5 0 0 0 0-5H18"/><path d="M4 22h16"/><path d="M10 14.66V17c0 .55-.45 1-1 1H8c-.55 0-1 .45-1 1v1c0 .55.45 1 1 1h8c.55 0 1-.45 1-1v-1c0-.55-.45-1-1-1h-1c-.55 0-1-.45-1-1v-2.34"/><path d="M18 2H6v7a6 6 0 0 0 12 0V2z"/></svg>
            </div>
            <h2>%s Wins!</h2>
            <div class="showdown-win-amount">+%d Chips</div>
            <div class="showdown-hand-desc">%s</div>
            <div class="showdown-countdown" data-countdown-until="%d">
                <span class="countdown-pulse">Next hand starts in <span data-countdown-value>%d</span>s</span>
            </div>
        </div>
    </div>`, winnerName, amount, handName, intermissionEndsAtMs, remainingSecondsFromMs(intermissionEndsAtMs))
}
