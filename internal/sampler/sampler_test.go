package sampler

import (
	"testing"
	"time"

	"codeberg.org/ubunatic/proctop/internal/proc"
)

func TestRecordCPUAndPeaks(t *testing.T) {
	r := New(100, 10) // 100 ticks/s
	t0 := time.Unix(1000, 0)

	if _, ok := r.Record(t0, []proc.Stat{{PID: 1, Ticks: 1000, RSS: 100}}); ok {
		t.Fatal("first record must only prime, not produce a sample")
	}

	// +50 ticks in 1s at 100 Hz = 50% CPU.
	s, ok := r.Record(t0.Add(time.Second), []proc.Stat{{PID: 1, Ticks: 1050, RSS: 200}})
	if !ok {
		t.Fatal("second record must produce a sample")
	}
	if s.CPUPct != 50 {
		t.Errorf("cpu = %v, want 50", s.CPUPct)
	}
	if s.RSS != 200 || s.Procs != 1 {
		t.Errorf("rss/procs = %v/%v, want 200/1", s.RSS, s.Procs)
	}

	// New child appears (pid 2 has no prev sample → its ticks don't count yet).
	s, ok = r.Record(t0.Add(2*time.Second), []proc.Stat{
		{PID: 1, Ticks: 1060, RSS: 100},
		{PID: 2, Ticks: 9999, RSS: 300},
	})
	if !ok || s.CPUPct != 10 {
		t.Errorf("cpu = %v, want 10 (new child ticks ignored)", s.CPUPct)
	}
	if s.RSS != 400 || s.Procs != 2 {
		t.Errorf("rss/procs = %v/%v, want 400/2", s.RSS, s.Procs)
	}

	if r.CPUMax.Value != 50 || !r.CPUMax.Time.Equal(t0.Add(time.Second)) {
		t.Errorf("cpu max = %+v, want 50 @ t0+1s", r.CPUMax)
	}
	if r.CPUMin.Value != 10 {
		t.Errorf("cpu min = %v, want 10", r.CPUMin.Value)
	}
	if r.MemMax.Value != 400 || r.MemMin.Value != 200 {
		t.Errorf("mem max/min = %v/%v, want 400/200", r.MemMax.Value, r.MemMin.Value)
	}
	if r.CPUAvg() != 30 {
		t.Errorf("cpu avg = %v, want 30", r.CPUAvg())
	}
	if r.MemAvg() != 300 {
		t.Errorf("mem avg = %v, want 300", r.MemAvg())
	}
	if r.Samples != 2 {
		t.Errorf("samples = %d, want 2", r.Samples)
	}
}

func TestHistoryBound(t *testing.T) {
	r := New(100, 3)
	t0 := time.Unix(1000, 0)
	for i := 0; i < 10; i++ {
		r.Record(t0.Add(time.Duration(i)*time.Second), []proc.Stat{{PID: 1, Ticks: uint64(i * 10)}})
	}
	if len(r.History) != 3 {
		t.Errorf("history len = %d, want 3", len(r.History))
	}
	if r.Samples != 9 {
		t.Errorf("samples = %d, want 9 (first record primes only)", r.Samples)
	}
}
