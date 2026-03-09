package scheduler

import (
	"path/filepath"
	"testing"

	"github.com/phekno/gobin/internal/config"
)

func TestDayMatch(t *testing.T) {
	tests := []struct {
		day  string
		days []string
		want bool
	}{
		{"mon", []string{"mon", "tue", "wed"}, true},
		{"fri", []string{"mon", "tue", "wed"}, false},
		{"Mon", []string{"mon"}, true}, // case insensitive
		{"sun", []string{}, false},
	}
	for _, tt := range tests {
		if got := dayMatch(tt.day, tt.days); got != tt.want {
			t.Errorf("dayMatch(%q, %v) = %v, want %v", tt.day, tt.days, got, tt.want)
		}
	}
}

func TestTimeInRange(t *testing.T) {
	tests := []struct {
		current, start, end string
		want                bool
	}{
		{"12:00", "08:00", "17:00", true},
		{"07:59", "08:00", "17:00", false},
		{"17:00", "08:00", "17:00", false}, // end is exclusive
		{"08:00", "08:00", "17:00", true},
		{"23:00", "22:00", "23:30", true},
	}
	for _, tt := range tests {
		if got := timeInRange(tt.current, tt.start, tt.end); got != tt.want {
			t.Errorf("timeInRange(%q, %q, %q) = %v, want %v", tt.current, tt.start, tt.end, got, tt.want)
		}
	}
}

func TestEvaluate(t *testing.T) {
	cfgMgr := config.NewManager(filepath.Join(t.TempDir(), "cfg.yaml"), &config.Config{
		Downloads: config.Downloads{SpeedLimitKbps: 100},
		Schedule: config.Schedule{
			Enabled: true,
			Rules: []config.ScheduleRule{
				{Days: []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}, Start: "00:00", End: "23:59", SpeedLimitKbps: 50000},
			},
		},
	})
	s := New(cfgMgr)
	s.evaluate()
	// The rule should match (covers all days and all times)
	if s.SpeedLimit() != 50000*1024 {
		t.Errorf("SpeedLimit = %d, want %d", s.SpeedLimit(), 50000*1024)
	}
}

func TestEvaluate_NoMatch(t *testing.T) {
	cfgMgr := config.NewManager(filepath.Join(t.TempDir(), "cfg.yaml"), &config.Config{
		Downloads: config.Downloads{SpeedLimitKbps: 200},
		Schedule: config.Schedule{
			Enabled: true,
			Rules:   []config.ScheduleRule{}, // No rules
		},
	})
	s := New(cfgMgr)
	s.evaluate()
	// Should fall back to global limit
	if s.SpeedLimit() != 200*1024 {
		t.Errorf("SpeedLimit = %d, want %d", s.SpeedLimit(), 200*1024)
	}
}

func TestSpeedLimit(t *testing.T) {
	s := &Scheduler{}
	if s.SpeedLimit() != 0 {
		t.Error("default should be 0 (unlimited)")
	}
	s.currentLimit.Store(50 * 1024)
	if s.SpeedLimit() != 50*1024 {
		t.Errorf("SpeedLimit = %d", s.SpeedLimit())
	}
}
