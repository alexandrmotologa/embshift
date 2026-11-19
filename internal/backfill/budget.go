package backfill

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/alexandrmotologa/embshift/internal/config"
)

// BudgetRegulator enforces token rate limits (TPM, RPM) and absolute dollar budget caps.
type BudgetRegulator struct {
	mu                       sync.Mutex
	cfg                      config.BackfillConfig
	pricePerMillionTokensUSD float64
	tokensUsed               int64
	maxBudgetUSD             float64
	tokenBucket              float64
	maxTokenBucket           float64
	tokensPerSecond          float64
	lastRefill               time.Time
}

// NewBudgetRegulator constructs a rate and spend controller.
func NewBudgetRegulator(cfg config.BackfillConfig, pricePerMillionUSD float64) *BudgetRegulator {
	tpm := float64(cfg.TokensPerMinute)
	if tpm <= 0 {
		tpm = 1000000.0 // Default 1M TPM
	}
	tps := tpm / 60.0

	return &BudgetRegulator{
		cfg:                      cfg,
		pricePerMillionTokensUSD: pricePerMillionUSD,
		maxBudgetUSD:             cfg.MaxBudgetUSD,
		tokenBucket:              tpm,
		maxTokenBucket:           tpm,
		tokensPerSecond:          tps,
		lastRefill:               time.Now(),
	}
}

// Acquire blocks until sufficient token quota is available or context cancels.
func (b *BudgetRegulator) Acquire(ctx context.Context, estimatedTokens int) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check dollar budget cap
	spent := (float64(b.tokensUsed) / 1000000.0) * b.pricePerMillionTokensUSD
	if b.maxBudgetUSD > 0 && spent >= b.maxBudgetUSD {
		return fmt.Errorf("budget exceeded: current spend $%.4f reached cap $%.2f", spent, b.maxBudgetUSD)
	}

	needed := float64(estimatedTokens)
	if needed <= 0 {
		needed = 100.0
	}

	for {
		now := time.Now()
		elapsed := now.Sub(b.lastRefill).Seconds()
		b.tokenBucket = math.Min(b.maxTokenBucket, b.tokenBucket+(elapsed*b.tokensPerSecond))
		b.lastRefill = now

		if b.tokenBucket >= needed {
			b.tokenBucket -= needed
			return nil
		}

		waitSec := (needed - b.tokenBucket) / b.tokensPerSecond
		waitDuration := time.Duration(waitSec * float64(time.Second))
		if waitDuration > 5*time.Second {
			waitDuration = 5 * time.Second
		}
		if waitDuration < 10*time.Millisecond {
			waitDuration = 10 * time.Millisecond
		}

		b.mu.Unlock()
		select {
		case <-ctx.Done():
			b.mu.Lock()
			return ctx.Err()
		case <-time.After(waitDuration):
		}
		b.mu.Lock()
	}
}

// RecordUsage updates total token consumption counters.
func (b *BudgetRegulator) RecordUsage(actualTokens int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tokensUsed += actualTokens
}

// GetStats returns current usage and spend metrics.
func (b *BudgetRegulator) GetStats() (tokens int64, spentUSD float64, isExhausted bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	spent := (float64(b.tokensUsed) / 1000000.0) * b.pricePerMillionTokensUSD
	exhausted := b.maxBudgetUSD > 0 && spent >= b.maxBudgetUSD
	return b.tokensUsed, spent, exhausted
}
