package telegram

import (
	"context"
	"sync"
	"time"
)

// Limiter implementa un token-bucket simple para respetar límites de Telegram.
// Global ~30 msg/s; por chat ~1 msg/s en envíos consecutivos.
type Limiter struct {
	mu       sync.Mutex
	global   *bucket
	perChat  map[string]*bucket
	chatRate time.Duration
}

type bucket struct {
	tokens float64
	max    float64
	rate   float64 // tokens por segundo
	last   time.Time
}

// NewLimiter crea un limitador con ~30 msg/s globales y 1 msg/s por chat.
// El balde global arranca con 1 token (no 30): sin ello la primera ráfaga
// dispararía 30 mensajes de golpe y provocaría 429.
func NewLimiter() *Limiter {
	now := time.Now()
	return &Limiter{
		global: &bucket{
			tokens: 1,
			max:    30,
			rate:   30,
			last:   now,
		},
		perChat:  make(map[string]*bucket),
		chatRate: time.Second,
	}
}

func (b *bucket) refill(now time.Time) {
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * b.rate
		if b.tokens > b.max {
			b.tokens = b.max
		}
		b.last = now
	}
}

func (b *bucket) take(now time.Time) time.Duration {
	b.refill(now)
	if b.tokens >= 1 {
		b.tokens--
		return 0
	}
	need := 1 - b.tokens
	wait := time.Duration(need/b.rate*float64(time.Second)) + time.Millisecond
	return wait
}

// Wait bloquea hasta que haya cupo global y por chat.
func (l *Limiter) Wait(ctx context.Context, chatID string) error {
	for {
		l.mu.Lock()
		now := time.Now()
		wait := l.global.take(now)
		if wait == 0 && chatID != "" {
			cb, ok := l.perChat[chatID]
			if !ok {
				cb = &bucket{tokens: 1, max: 1, rate: 1, last: now}
				l.perChat[chatID] = cb
			}
			if w := cb.take(now); w > wait {
				wait = w
			}
		}
		l.mu.Unlock()
		if wait == 0 {
			return nil
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}
