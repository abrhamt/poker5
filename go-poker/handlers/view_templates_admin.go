package handlers

import (
	"fmt"
	"html"
	"time"

	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/services"
)

func renderAdminDashboardHTML(earnings services.EarningsSummary, settings repository.SiteSetting, users []repository.User, txs []adminTxRow, pendingDeposits []repository.ListBankDepositsForReviewRow) string {
	realDepositsOn, realDepositsOff := "", " selected"
	if settings.RealDepositsEnabled {
		realDepositsOn, realDepositsOff = " selected", ""
	}

	percentSelected := ""
	sbSelected := ""
	if settings.RakeMode == services.RakeModeSmallBlind {
		sbSelected = " selected"
	} else {
		percentSelected = " selected"
	}

	content := fmt.Sprintf(`
    <div class="admin-container">
        <div class="admin-header">
            <h1>Admin Dashboard</h1>
        </div>

        <div class="earnings-grid">
            <div class="earnings-card highlight">
                <span class="earnings-label">All-Time House Balance</span>
                <span class="earnings-val">%d ETB</span>
            </div>
            <div class="earnings-card">
                <span class="earnings-label">Rake Today</span>
                <span class="earnings-val">%d ETB</span>
            </div>
            <div class="earnings-card">
                <span class="earnings-label">Rake This Week</span>
                <span class="earnings-val">%d ETB</span>
            </div>
            <div class="earnings-card">
                <span class="earnings-label">Rake This Month</span>
                <span class="earnings-val">%d ETB</span>
            </div>
            <div class="earnings-card muted">
                <span class="earnings-label">Referrals Paid Today</span>
                <span class="earnings-val">%d ETB</span>
            </div>
            <div class="earnings-card muted">
                <span class="earnings-label">Referrals Paid This Week</span>
                <span class="earnings-val">%d ETB</span>
            </div>
            <div class="earnings-card muted">
                <span class="earnings-label">Referrals Paid This Month</span>
                <span class="earnings-val">%d ETB</span>
            </div>
        </div>

        <div class="admin-grid">
            <section class="admin-card">
                <h2>Site Settings</h2>
                <form action="/api/admin/settings" method="post"
                      hx-post="/api/admin/settings" hx-target="#settings-error" class="settings-form"
                      hx-confirm="Save these site settings? Rake mode, percentages, and countdown apply to every table immediately.">
                    <div id="settings-error"></div>
                    <div class="form-group">
                        <label for="rake_mode">Rake Mode</label>
                        <select id="rake_mode" name="rake_mode">
                            <option value="percentage"%s>Percentage of Pot</option>
                            <option value="small_blind"%s>Site Always Takes the Small Blind</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label for="rake_percentage">Rake Percentage (0-%g%%, used in percentage mode)</label>
                        <input type="number" id="rake_percentage" name="rake_percentage" min="%g" max="%g" step="0.1" value="%g">
                    </div>
                    <div class="form-group">
                        <label for="referral_percentage_pct_mode">Referral %% of Winner (percentage mode)</label>
                        <input type="number" id="referral_percentage_pct_mode" name="referral_percentage_pct_mode" min="%g" max="%g" step="0.1" value="%g">
                    </div>
                    <div class="form-group">
                        <label for="referral_percentage_sb_mode">Referral %% of Winner (small-blind mode)</label>
                        <input type="number" id="referral_percentage_sb_mode" name="referral_percentage_sb_mode" min="%g" max="%g" step="0.1" value="%g">
                    </div>
                    <div class="form-group">
                        <label for="countdown_seconds">Countdown Before Hand Starts (seconds)</label>
                        <input type="number" id="countdown_seconds" name="countdown_seconds" min="%d" max="%d" value="%d">
                    </div>
                    <div class="form-group">
                        <label for="real_deposits_enabled">Deposits</label>
                        <select id="real_deposits_enabled" name="real_deposits_enabled">
                            <option value="false"%s>Play money — any amount, credited instantly</option>
                            <option value="true"%s>Real money — require a CBE transfer receipt</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label for="deposit_account_name">Deposit Account Name (must match the CBE receipt exactly)</label>
                        <input type="text" id="deposit_account_name" name="deposit_account_name" maxlength="%d" value="%s">
                    </div>
                    <div class="form-group">
                        <label for="deposit_account_number">Deposit Account Number (full number, as on your CBE account)</label>
                        <input type="text" id="deposit_account_number" name="deposit_account_number" maxlength="%d" value="%s">
                    </div>
                    <button type="submit" class="btn-gold btn-block">Save Settings</button>
                </form>
            </section>

            <section class="admin-card">
                <h2>Users</h2>
                <div class="form-row">
                    <input type="text" name="query" placeholder="Search username or phone..."
                           hx-get="/api/admin/users" hx-trigger="keyup changed delay:300ms" hx-target="#admin-users-table">
                </div>
                <div id="admin-users-table" class="table-responsive">
                    %s
                </div>
            </section>

            <section class="admin-card admin-card-wide">
                <h2>Bank Deposits Awaiting Review</h2>
                <p class="muted">Receipts pasted while the bank verifier was unreachable. These are re-checked automatically every few minutes and credit themselves once the verifier answers — what stays here needed a person. <strong>Verify &amp; credit</strong> re-checks the receipt against the bank and never credits on trust alone. <strong>Credit manually</strong> is the escape hatch for a receipt the verifier simply cannot read: it credits the amount you type with no bank check at all, so read the amount off the receipt itself, and enter the FT reference whenever you can see it — that is what stops the same transfer being credited twice through its other link.</p>
                <div id="admin-deposits-table" class="table-responsive">
                    %s
                </div>
            </section>

            <section class="admin-card admin-card-wide">
                <h2>Transaction History</h2>
                <form class="tx-filter-row" hx-get="/api/admin/transactions" hx-target="#admin-tx-table"
                      hx-trigger="keyup changed delay:300ms from:input, change from:select">
                    <input type="text" name="query" placeholder="Search by username...">
                    <select name="type">
                        <option value="">All Types</option>
                        <option value="deposit">Deposit</option>
                        <option value="buy_in">Buy-in</option>
                        <option value="cash_out">Cash-out</option>
                        <option value="winner_payout">Winner Payout</option>
                        <option value="site_rake">Site Rake</option>
                        <option value="referral_commission">Referral Commission</option>
                    </select>
                </form>
                <div id="admin-tx-table" class="table-responsive">
                    %s
                </div>
            </section>
        </div>
    </div>`,
		earnings.HouseBalance,
		earnings.RakeToday,
		earnings.RakeWeek,
		earnings.RakeMonth,
		earnings.ReferralPaidToday,
		earnings.ReferralPaidWeek,
		earnings.ReferralPaidMonth,
		percentSelected, sbSelected,
		services.MaxRakePercentage, services.MinRakePercentage, services.MaxRakePercentage, settings.RakePercentage,
		services.MinReferralPercentage, services.MaxReferralPercentage, settings.ReferralPercentagePctMode,
		services.MinReferralPercentage, services.MaxReferralPercentage, settings.ReferralPercentageSbMode,
		services.MinCountdownSeconds, services.MaxCountdownSeconds, settings.CountdownSeconds,
		realDepositsOff, realDepositsOn,
		services.MaxDepositAccountName, html.EscapeString(settings.DepositAccountName),
		services.MaxDepositAccountNumber, html.EscapeString(settings.DepositAccountNumber),
		renderAdminUsersTableHTML(users),
		renderAdminDepositsTableHTML(pendingDeposits, ""),
		renderAdminTransactionsTableHTML(txs),
	)

	return renderBaseLayout("Admin", content, "admin")
}

func renderAdminUsersTableHTML(users []repository.User) string {
	rows := ""
	if len(users) == 0 {
		rows = `<tr><td colspan="5" class="text-center">No users found.</td></tr>`
	}
	for _, u := range users {
		roleBadge := "badge-info"
		if u.Role == "admin" {
			roleBadge = "badge-gold"
		}
		rows += fmt.Sprintf(`
        <tr>
            <td>%s</td>
            <td>%s</td>
            <td>%d ETB</td>
            <td><span class="badge %s">%s</span></td>
            <td>%s</td>
        </tr>`, u.Username, u.PhoneNumber, u.Wallet, roleBadge, u.Role, u.CreatedAt.Format("Jan 02, 2006"))
	}

	return fmt.Sprintf(`
    <table class="data-table">
        <thead><tr><th>Username</th><th>Phone</th><th>Wallet</th><th>Role</th><th>Joined</th></tr></thead>
        <tbody>%s</tbody>
    </table>`, rows)
}

// adminTxRow is a flattened view shared by both the "list recent" and
// "search by username" transaction queries, so the table renderer doesn't
// need to care which one produced the rows.
type adminTxRow struct {
	Username      string
	Type          string
	Amount        int64
	Reason        string
	TransactionID string
	CreatedAt     string
}

func renderAdminTransactionsTableHTML(rows []adminTxRow) string {
	body := ""
	if len(rows) == 0 {
		body = `<tr><td colspan="5" class="text-center">No transactions found.</td></tr>`
	}
	for _, t := range rows {
		badgeClass := "badge-info"
		switch t.Type {
		case "deposit", "cash_out", "winner_payout":
			badgeClass = "badge-success"
		case "buy_in":
			badgeClass = "badge-warning"
		case "site_rake":
			badgeClass = "badge-gold"
		case "referral_commission":
			badgeClass = "badge-gold"
		}
		body += fmt.Sprintf(`
        <tr>
            <td>%s</td>
            <td><span class="badge %s">%s</span></td>
            <td>%d ETB</td>
            <td><span class="tx-code">%s</span></td>
            <td>%s</td>
        </tr>`, t.Username, badgeClass, t.Type, t.Amount, t.TransactionID, t.CreatedAt)
	}

	return fmt.Sprintf(`
    <table class="data-table">
        <thead><tr><th>User</th><th>Type</th><th>Amount</th><th>Tx ID</th><th>Date</th></tr></thead>
        <tbody>%s</tbody>
    </table>`, body)
}

// renderAdminDepositsTableHTML draws the review queue. Every value here except
// the id came from a player's paste or from the bank's response, so all of it
// is escaped — this table is the one place in the dashboard that renders text
// this app never wrote.
func renderAdminDepositsTableHTML(rows []repository.ListBankDepositsForReviewRow, notice string) string {
	banner := ""
	if notice != "" {
		banner = fmt.Sprintf(`<div class="info-badge">%s</div>`, html.EscapeString(notice))
	}

	body := ""
	if len(rows) == 0 {
		body = `<tr><td colspan="4" class="text-center">Nothing waiting for review.</td></tr>`
	}
	for _, row := range rows {
		d := row.BankDeposit
		tried := "not yet re-checked"
		switch {
		case d.Attempts >= services.MaxQueueAttempts:
			tried = fmt.Sprintf("%d attempts — automatic retry gave up", d.Attempts)
		case d.Attempts > 0:
			tried = fmt.Sprintf("%d automatic attempts", d.Attempts)
		}
		lastNote := ""
		if d.Note != "" {
			lastNote = fmt.Sprintf(`<br><span class="muted">%s</span>`, html.EscapeString(d.Note))
		}
		body += fmt.Sprintf(`
        <tr>
            <td>%s</td>
            <td class="break-all"><a href="%s" target="_blank" rel="noopener noreferrer">%s</a></td>
            <td>%s<br><span class="muted">%s</span>%s</td>
            <td class="text-right">
                <div class="deposit-actions">
                    <button class="btn-gold btn-sm" hx-post="/api/admin/deposits/%d/approve" hx-target="#admin-deposits-table"
                            hx-confirm="Re-check this receipt with the bank and credit it if it passes?">Verify &amp; credit</button>
                    <button class="btn-outline btn-sm" hx-post="/api/admin/deposits/%d/reject" hx-target="#admin-deposits-table"
                            hx-confirm="Reject this receipt? The player is not credited.">Reject</button>
                </div>
                <div class="deposit-manual">
                    <input type="number" id="manual-amount-%d" name="amount" min="%d" max="%d" step="1" placeholder="Amount (ETB)">
                    <input type="text" id="manual-reference-%d" name="reference" maxlength="64" placeholder="FT reference (recommended)">
                    <button class="btn-outline btn-sm" hx-post="/api/admin/deposits/%d/credit"
                            hx-include="#manual-amount-%d,#manual-reference-%d" hx-target="#admin-deposits-table"
                            hx-confirm="Credit this player the amount you typed, without any bank check?">Credit manually</button>
                </div>
            </td>
        </tr>`,
			html.EscapeString(row.Username),
			html.EscapeString(d.ReceiptUrl),
			html.EscapeString(d.ReceiptUrl),
			d.CreatedAt.UTC().Format(time.RFC822),
			tried,
			lastNote,
			d.ID, d.ID,
			d.ID, services.MinReceiptDeposit, services.MaxManualDeposit,
			d.ID,
			d.ID, d.ID, d.ID)
	}

	return fmt.Sprintf(`%s
    <table class="data-table">
        <thead><tr><th>Player</th><th>Receipt link</th><th>Submitted</th><th class="text-right">Action</th></tr></thead>
        <tbody>%s</tbody>
    </table>`, banner, body)
}
