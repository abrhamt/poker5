package handlers

import (
	"fmt"

	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/services"
)

// renderPublicRoomsSection renders the live public-room list. It is
// self-contained (carries its own hx-trigger polling attributes on its root
// element) so the exact same markup can be embedded in the full lobby page
// and returned as-is by the /lobby/rooms poll endpoint.
func renderPublicRoomsSection(publicRooms []repository.PokerRoom, live map[string]services.TableLiveSummary) string {
	roomsByTier := map[int32][]repository.PokerRoom{}
	for _, r := range publicRooms {
		roomsByTier[r.SmallBlind] = append(roomsByTier[r.SmallBlind], r)
	}

	tiersHTML := `<div class="tier-grid">`
	for _, tier := range services.PublicBlindTiers {
		rooms := roomsByTier[tier.SmallBlind]
		roomCards := ""
		if len(rooms) == 0 {
			roomCards = `<p class="tier-empty">No active tables yet — quick join to start one.</p>`
		} else {
			for _, r := range rooms {
				summary := live[r.RoomCode]

				statusHTML := ""
				if summary.CountdownActive {
					statusHTML = fmt.Sprintf(`
                    <div class="room-status-badge room-status-countdown" data-countdown-until="%d">
                        Starting in <span data-countdown-value>%d</span>s
                    </div>`, summary.CountdownEndsAtMs, remainingSecondsFromMs(summary.CountdownEndsAtMs))
				} else if summary.GameStarted {
					statusHTML = `<div class="room-status-badge room-status-live">Hand in progress</div>`
				}

				roomCards += fmt.Sprintf(`
                <div class="room-card">
                    <div class="room-card-header">
                        <span class="room-tag public">Public</span>
                        <span class="room-code">#%s</span>
                    </div>
                    <h3 class="room-title">%s</h3>
                    <div class="room-meta">
                        <div class="meta-item">
                            <span class="meta-label">Buy-in</span>
                            <span class="meta-val">%d ETB</span>
                        </div>
                        <div class="meta-item">
                            <span class="meta-label">Blinds</span>
                            <span class="meta-val">%d / %d</span>
                        </div>
                        <div class="meta-item">
                            <span class="meta-label">Seated</span>
                            <span class="meta-val">%d / %d</span>
                        </div>
                    </div>
                    %s
                    <a href="/table/%s" class="btn-primary btn-block">Enter Table</a>
                </div>`, r.RoomCode, r.RoomName, r.BuyIn, r.SmallBlind, r.BigBlind, summary.PlayerCount, r.MaxPlayers, statusHTML, r.RoomCode)
			}
		}

		tiersHTML += fmt.Sprintf(`
        <div class="tier-card">
            <div class="tier-card-header">
                <h3>%d / %d Blinds</h3>
                <form hx-post="/api/rooms/quick-join" class="tier-quick-join">
                    <input type="hidden" name="small_blind" value="%d">
                    <button type="submit" class="btn-gold btn-sm">Quick Join</button>
                </form>
            </div>
            <div class="tier-room-list">%s</div>
        </div>`, tier.SmallBlind, tier.BigBlind, tier.SmallBlind, roomCards)
	}
	tiersHTML += `</div>`

	return fmt.Sprintf(`
    <section class="lobby-section featured-section" id="public-rooms-section" hx-trigger="every 5s" hx-get="/lobby/rooms" hx-swap="outerHTML">
        <div class="section-header">
            <h2>Public Tables</h2>
        </div>
        %s
    </section>`, tiersHTML)
}

func renderLobbyHTML(publicRooms []repository.PokerRoom, live map[string]services.TableLiveSummary, activeRoomCode string) string {
	resumeHTML := ""
	if activeRoomCode != "" {
		resumeHTML = fmt.Sprintf(`
        <a href="/table/%s" class="resume-table-banner">
            <span class="resume-table-text">You're still seated at table <strong>#%s</strong> — resume to keep playing or cash out.</span>
            <span class="btn-gold btn-sm">Resume Table</span>
        </a>`, activeRoomCode, activeRoomCode)
	}

	return fmt.Sprintf(`
    <div class="lobby-container">
        <div class="lobby-hero">
            <div class="hero-content">
                <h1>Golden Poker Arena</h1>
                <p>Live Multiplayer Texas Hold'em</p>
            </div>
        </div>

        %s

        <div class="action-cards-grid">
            <div class="action-card featured-card">
                <div class="card-icon-box">
                    <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>
                </div>
                <h3>Private Room</h3>
                <p>Create a dedicated private table with an instant 5-digit code to share. You choose the blinds (small blind &ge; 10) and seat count — everyone joins with their full wallet balance, as long as it clears the big blind.</p>
                <form hx-post="/api/rooms/create-private" hx-target="#private-room-error" class="room-form">
                    <div id="private-room-error"></div>
                    <div class="form-row">
                        <label for="private-small-blind">Small Blind</label>
                        <input type="number" id="private-small-blind" name="small_blind" min="10" step="5" value="10" required>
                    </div>
                    <div class="form-row">
                        <label for="private-max-players">Seats</label>
                        <select id="private-max-players" name="max_players">
                            <option value="2">2 players</option>
                            <option value="4">4 players</option>
                            <option value="6" selected>6 players</option>
                            <option value="9">9 players</option>
                        </select>
                    </div>
                    <button type="submit" class="btn-gold btn-block">Create Private Room</button>
                </form>
            </div>

            <div class="action-card featured-card">
                <div class="card-icon-box">
                    <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 2l-2 2m-1.5 1.5L14 9l-1.5-1.5L10 10l1.5 1.5L8 15H5v3h3v-3l1.5-1.5L11 15l2.5-2.5-1.5-1.5 3.5-3.5 1.5 1.5 2-2z"/></svg>
                </div>
                <h3>Join by Code</h3>
                <p>Enter the 5-digit room code to join an active private game.</p>
                <form hx-post="/api/rooms/join-code" hx-target="#code-error" class="room-form">
                    <div id="code-error"></div>
                    <div class="form-row">
                        <input type="text" name="code" placeholder="5-Digit Code" maxlength="5" pattern="[0-9]{5}" required class="code-input">
                    </div>
                    <button type="submit" class="btn-primary btn-block">Join Table</button>
                </form>
            </div>
        </div>

        %s
    </div>`, resumeHTML, renderPublicRoomsSection(publicRooms, live))
}

func renderWalletHTML(user *repository.User, txs []repository.Transaction) string {
	txRows := ""
	if len(txs) == 0 {
		txRows = `<tr><td colspan="4" class="text-center">No transactions recorded yet.</td></tr>`
	} else {
		for _, t := range txs {
			badgeClass := "badge-info"
			switch t.Type {
			case "deposit":
				badgeClass = "badge-success"
			case "buy_in":
				badgeClass = "badge-warning"
			case "cash_out", "winner_payout":
				badgeClass = "badge-success"
			case "referral_commission":
				badgeClass = "badge-gold"
			}

			txRows += fmt.Sprintf(`
            <tr>
                <td><span class="tx-code">%s</span></td>
                <td><span class="badge %s">%s</span></td>
                <td class="tx-amount">%d ETB</td>
                <td class="tx-date">%s</td>
            </tr>`, t.TransactionID, badgeClass, t.Type, t.Amount, t.CreatedAt.Format("Jan 02, 15:04"))
		}
	}

	return fmt.Sprintf(`
    <div class="wallet-container">
        <div class="wallet-grid">
            <div class="wallet-card balance-card">
                <h2>Player Wallet</h2>
                <div class="balance-display">
                    <span class="balance-val">%d</span>
                    <span class="currency">ETB</span>
                </div>
                <p class="balance-hint">Funds are converted to chips 1:1 during table buy-ins and settled back on cash-out.</p>
                <div class="referral-box">
                    <span class="ref-label">Referral Code:</span>
                    <span class="ref-code">%s</span>
                </div>
            </div>

            <div class="wallet-card deposit-card">
                <h2>Deposit ETB</h2>
                <form hx-post="/api/wallet/deposit" hx-target="#deposit-feedback" class="deposit-form">
                    <div id="deposit-feedback"></div>
                    <div class="preset-amounts">
                        <button type="button" class="preset-btn" onclick="document.getElementById('deposit-amount').value = 50">50 ETB</button>
                        <button type="button" class="preset-btn" onclick="document.getElementById('deposit-amount').value = 100">100 ETB</button>
                        <button type="button" class="preset-btn" onclick="document.getElementById('deposit-amount').value = 250">250 ETB</button>
                        <button type="button" class="preset-btn" onclick="document.getElementById('deposit-amount').value = 500">500 ETB</button>
                        <button type="button" class="preset-btn" onclick="document.getElementById('deposit-amount').value = 1000">1,000 ETB</button>
                        <button type="button" class="preset-btn" onclick="document.getElementById('deposit-amount').value = 2000">2,000 ETB</button>
                    </div>
                    <div class="form-group">
                        <label for="deposit-amount">Amount (ETB)</label>
                        <input type="number" step="1" min="1" max="100000" id="deposit-amount" name="amount" value="100" required>
                    </div>
                    <button type="submit" class="btn-gold btn-block">Confirm Deposit</button>
                </form>
            </div>
        </div>

        <section class="wallet-history">
            <h2>Transaction Log</h2>
            <div class="table-responsive">
                <table class="data-table">
                    <thead>
                        <tr>
                            <th>Transaction ID</th>
                            <th>Type</th>
                            <th>Amount</th>
                            <th>Date</th>
                        </tr>
                    </thead>
                    <tbody>
                        %s
                    </tbody>
                </table>
            </div>
        </section>
    </div>`, user.Wallet, user.ReferralCode, txRows)
}
