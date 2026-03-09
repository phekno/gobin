// Package scheduler provides bandwidth throttling and time-based speed rules.
package scheduler

import (
	"context"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/phekno/gobin/internal/config"
)

// Scheduler manages speed limits based on time-of-day rules.
type Scheduler struct {
	cfgMgr       *config.Manager
	currentLimit atomic.Int64 // bytes per second, 0 = unlimited
}

// New creates a scheduler.
func New(cfgMgr *config.Manager) *Scheduler {
	return &Scheduler{cfgMgr: cfgMgr}
}

// Run starts the scheduler. It periodically checks time-based rules
// and updates the speed limit. Blocks until context is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	cfg := s.cfgMgr.Get()

	// Apply global speed limit from config
	if cfg.Downloads.SpeedLimitKbps > 0 {
		s.currentLimit.Store(int64(cfg.Downloads.SpeedLimitKbps) * 1024)
		slog.Info("global speed limit set", "kbps", cfg.Downloads.SpeedLimitKbps)
	}

	if !cfg.Schedule.Enabled {
		slog.Info("scheduling disabled")
		return
	}

	slog.Info("scheduler started", "rules", len(cfg.Schedule.Rules))

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Check immediately
	s.evaluate()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.evaluate()
		}
	}
}

// SpeedLimit returns the current speed limit in bytes/sec. 0 = unlimited.
func (s *Scheduler) SpeedLimit() int64 {
	return s.currentLimit.Load()
}

// evaluate checks current time against schedule rules and sets the limit.
func (s *Scheduler) evaluate() {
	cfg := s.cfgMgr.Get()
	now := time.Now()
	dayName := strings.ToLower(now.Weekday().String()[:3])
	timeStr := now.Format("15:04")

	for _, rule := range cfg.Schedule.Rules {
		if !dayMatch(dayName, rule.Days) {
			continue
		}
		if timeInRange(timeStr, rule.Start, rule.End) {
			limit := int64(rule.SpeedLimitKbps) * 1024
			if s.currentLimit.Load() != limit {
				s.currentLimit.Store(limit)
				slog.Info("schedule rule active",
					"limit_kbps", rule.SpeedLimitKbps,
					"start", rule.Start,
					"end", rule.End,
				)
			}
			return
		}
	}

	// No rule matches — use global limit (or unlimited)
	globalLimit := int64(cfg.Downloads.SpeedLimitKbps) * 1024
	if s.currentLimit.Load() != globalLimit {
		s.currentLimit.Store(globalLimit)
		if globalLimit > 0 {
			slog.Info("reverted to global speed limit", "kbps", cfg.Downloads.SpeedLimitKbps)
		} else {
			slog.Debug("speed limit: unlimited")
		}
	}
}

func dayMatch(day string, days []string) bool {
	for _, d := range days {
		if strings.EqualFold(d, day) {
			return true
		}
	}
	return false
}

func timeInRange(current, start, end string) bool {
	return current >= start && current < end
}
