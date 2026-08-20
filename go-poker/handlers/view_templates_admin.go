package handlers

import (
	"fmt"

	"github.com/zuse/poker5/go-poker/repository"
	"github.com/zuse/poker5/go-poker/services"
)

func renderAdminDashboardHTML(earnings services.EarningsSummary, settings repository.SiteSetting, users []repository.User, txs []adminTxRow) string {
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
                <form hx-post="/api/admin/settings" hx-target="#settings-error" class="settings-form"
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
		renderAdminUsersTableHTML(users),
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
