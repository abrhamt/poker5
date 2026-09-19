package services

import (
	"context"
	"log"
	"time"

	"github.com/zuse/poker5/go-poker/repository"
)

func (s *GatewayDepositService) StartReconciler(ctx context.Context, interval time.Duration) (stop func()) {
	if interval <= 0 {
		interval = GatewayReconcileEvery
	}
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.RunReconcile(ctx)
			}
		}
	}()

	return cancel
}

type ReconcileResult struct {
	Credited int
	Closed   int
	Pending  int
	Errors   int
}

func (s *GatewayDepositService) RunReconcile(ctx context.Context) ReconcileResult {
	var result ReconcileResult
	if !s.Configured() {
		return result
	}

	rows, err := s.q.ListGatewayDepositsPendingSince(ctx, repository.ListGatewayDepositsPendingSinceParams{
		Status:    GatewayDepositStatusPending,
		CreatedAt: time.Now().Add(-GatewayReconcileAfter),
		Limit:     gatewayReconcileBatch,
	})
	if err != nil {
		log.Printf("gateway reconcile: could not list pending deposits: %v", err)
		return result
	}

	for _, row := range rows {
		select {
		case <-ctx.Done():
			return result
		default:
		}

		payment, err := s.router.FetchPayment(ctx, row.RouterPaymentID)
		if err != nil {
			log.Printf("gateway reconcile: %s: %v", row.Reference, err)
			result.Errors++
			continue
		}
		event, ok := routerEventForStatus(payment.Status)
		if !ok {
			result.Pending++
			continue
		}
		if err := s.ApplyEvent(ctx, event, RouterEventData{
			ID:        payment.ID,
			Reference: row.Reference,
			Status:    payment.Status,
			Amount:    payment.Amount,
		}); err != nil {
			log.Printf("gateway reconcile: apply %s to %s: %v", event, row.Reference, err)
			result.Errors++
			continue
		}
		if event == RouterEventSucceeded {
			result.Credited++
		} else {
			result.Closed++
		}
	}

	if result.Credited > 0 || result.Closed > 0 || result.Errors > 0 {
		log.Printf("gateway reconcile: %d credited, %d closed, %d still pending, %d errors",
			result.Credited, result.Closed, result.Pending, result.Errors)
	}
	return result
}

func routerEventForStatus(status string) (string, bool) {
	switch status {
	case "succeeded":
		return RouterEventSucceeded, true
	case "failed":
		return RouterEventFailed, true
	case "cancelled":
		return RouterEventCancelled, true
	case "expired":
		return RouterEventExpired, true
	default:
		return "", false
	}
}
