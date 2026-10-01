package services

import (
	"context"
	"time"

	"github.com/zuse/poker5/go-poker/repository"
)

const AdminListPageSize = 25

type EarningsSummary struct {
	HouseBalance      int64
	RakeToday         int64
	RakeWeek          int64
	RakeMonth         int64
	ReferralPaidToday int64
	ReferralPaidWeek  int64
	ReferralPaidMonth int64
}

type AdminService struct {
	q      *repository.Queries
	wallet *WalletService
}

func NewAdminService(q *repository.Queries, wallet *WalletService) *AdminService {
	return &AdminService{q: q, wallet: wallet}
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func startOfWeek(t time.Time) time.Time {
	day := startOfDay(t)
	offset := (int(day.Weekday()) + 6) % 7 // Monday = start of week
	return day.AddDate(0, 0, -offset)
}

func startOfMonth(t time.Time) time.Time {
	y, m, _ := t.UTC().Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
}

func (s *AdminService) GetEarnings(ctx context.Context) (EarningsSummary, error) {
	summary := EarningsSummary{}

	houseID, err := s.wallet.EnsureHouseAccount(ctx)
	if err != nil {
		return summary, err
	}
	house, err := s.q.GetUserByID(ctx, houseID)
	if err != nil {
		return summary, err
	}
	summary.HouseBalance = house.Wallet

	now := time.Now()
	sums := []struct {
		txType string
		since  time.Time
		dest   *int64
	}{
		{TxTypeSiteRake, startOfDay(now), &summary.RakeToday},
		{TxTypeSiteRake, startOfWeek(now), &summary.RakeWeek},
		{TxTypeSiteRake, startOfMonth(now), &summary.RakeMonth},
		{TxTypeReferral, startOfDay(now), &summary.ReferralPaidToday},
		{TxTypeReferral, startOfWeek(now), &summary.ReferralPaidWeek},
		{TxTypeReferral, startOfMonth(now), &summary.ReferralPaidMonth},
	}
	for _, item := range sums {
		total, err := s.q.SumTransactionAmountByTypeSince(ctx, repository.SumTransactionAmountByTypeSinceParams{
			Type:      item.txType,
			CreatedAt: item.since,
		})
		if err != nil {
			return summary, err
		}
		*item.dest = total
	}

	return summary, nil
}

// SearchUsers pages through accounts whose username or phone contains query;
// an empty query lists everyone. The total is for the pager.
func (s *AdminService) SearchUsers(ctx context.Context, query string, page int) ([]repository.User, int64, error) {
	if page < 1 {
		page = 1
	}
	offset := int32((page - 1) * AdminListPageSize)
	if query == "" {
		total, err := s.q.CountUsers(ctx)
		if err != nil {
			return nil, 0, err
		}
		users, err := s.q.ListUsers(ctx, repository.ListUsersParams{Limit: AdminListPageSize, Offset: offset})
		return users, total, err
	}

	like := "%" + query + "%"
	total, err := s.q.CountSearchUsers(ctx, repository.CountSearchUsersParams{Username: like, PhoneNumber: like})
	if err != nil {
		return nil, 0, err
	}
	users, err := s.q.SearchUsers(ctx, repository.SearchUsersParams{
		Username:    like,
		PhoneNumber: like,
		Limit:       AdminListPageSize,
		Offset:      offset,
	})
	return users, total, err
}

// UserDetail is everything the admin's player page shows at once.
type UserDetail struct {
	User         repository.User
	Transactions []repository.Transaction
}

func (s *AdminService) GetUser(ctx context.Context, userID int64) (UserDetail, error) {
	user, err := s.q.GetUserByID(ctx, userID)
	if err != nil {
		return UserDetail{}, err
	}
	txs, err := s.q.ListTransactionsByUserID(ctx, repository.ListTransactionsByUserIDParams{
		UserID: userID,
		Limit:  AdminListPageSize,
	})
	if err != nil {
		return UserDetail{}, err
	}
	return UserDetail{User: user, Transactions: txs}, nil
}

// FilterTransactions lists recent transactions, optionally narrowed by a
// username substring and/or an exact transaction type. Empty strings mean
// "no filter" on that dimension.
func (s *AdminService) FilterTransactions(ctx context.Context, username, txType string, page int) ([]repository.FilterTransactionsRow, int64, error) {
	if page < 1 {
		page = 1
	}
	like := "%" + username + "%"
	total, err := s.q.CountFilterTransactions(ctx, repository.CountFilterTransactionsParams{Username: like, TypeFilter: txType})
	if err != nil {
		return nil, 0, err
	}
	rows, err := s.q.FilterTransactions(ctx, repository.FilterTransactionsParams{
		Username:   like,
		TypeFilter: txType,
		RowLimit:   AdminListPageSize,
		RowOffset:  int32((page - 1) * AdminListPageSize),
	})
	return rows, total, err
}
