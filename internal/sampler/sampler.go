// Package sampler turns raw /proc tick counters into a per-interval time
// series of CPU%% and memory usage for a process tree, and tracks running
// extremes and averages for the exit summary.
package sampler

import (
	"time"

	"codeberg.org/ubunatic/proctop/internal/proc"
)

// Sample is one row of the time series.
type Sample struct {
	Time   time.Time
	CPUPct float64 // percent of one core, summed over the tree (may exceed 100)
	RSS    int64   // bytes, summed over the tree
	Procs  int     // number of live processes in the tree
}

// Extreme records a value together with the time it was observed.
type Extreme struct {
	Value float64
	Time  time.Time
}

// Annotation is a user note attached to a span of samples. It is keyed by
// sample timestamps (never by graph column), so it stays correct while the
// history scrolls on; Start == End marks a single sample.
type Annotation struct {
	Start time.Time
	End   time.Time
	Note  string
}

// Covers reports whether t lies within the annotation span (inclusive).
func (a Annotation) Covers(t time.Time) bool {
	return !t.Before(a.Start) && !t.After(a.End)
}

// Recorder computes CPU%% from tick deltas between successive Record calls
// and maintains a bounded history plus running min/max/avg statistics.
type Recorder struct {
	ClkTck      int64
	HistorySize int

	History []Sample

	CPUMax, CPUMin Extreme
	MemMax, MemMin Extreme
	Start, End     time.Time
	Samples        int
	Annotations    []Annotation

	cpuSum float64
	memSum float64

	prevTicks map[int]uint64
	prevTime  time.Time
}

// New returns a Recorder using the given clock-tick rate and history bound.
func New(clkTck int64, historySize int) *Recorder {
	return &Recorder{ClkTck: clkTck, HistorySize: historySize}
}

// Record ingests one scan of the process tree taken at now. The first call
// only primes the tick counters and returns ok=false; every later call
// returns the computed sample and updates history and statistics.
func (r *Recorder) Record(now time.Time, tree []proc.Stat) (Sample, bool) {
	ticks := make(map[int]uint64, len(tree))
	var rss int64
	for _, st := range tree {
		ticks[st.PID] = st.Ticks
		rss += st.RSS
	}
	prev, prevTime := r.prevTicks, r.prevTime
	r.prevTicks, r.prevTime = ticks, now
	if prev == nil {
		return Sample{}, false
	}

	var delta uint64
	for pid, cur := range ticks {
		if old, ok := prev[pid]; ok && cur >= old {
			delta += cur - old
		}
	}
	dt := now.Sub(prevTime).Seconds()
	if dt <= 0 {
		return Sample{}, false
	}
	s := Sample{
		Time:   now,
		CPUPct: float64(delta) / float64(r.ClkTck) / dt * 100,
		RSS:    rss,
		Procs:  len(tree),
	}
	r.push(s)
	return s, true
}

func (r *Recorder) push(s Sample) {
	r.History = append(r.History, s)
	if len(r.History) > r.HistorySize {
		r.History = r.History[len(r.History)-r.HistorySize:]
	}
	if r.Samples == 0 {
		r.Start = s.Time
		r.CPUMax, r.CPUMin = Extreme{s.CPUPct, s.Time}, Extreme{s.CPUPct, s.Time}
		r.MemMax, r.MemMin = Extreme{float64(s.RSS), s.Time}, Extreme{float64(s.RSS), s.Time}
	}
	r.End = s.Time
	r.Samples++
	r.cpuSum += s.CPUPct
	r.memSum += float64(s.RSS)
	if s.CPUPct > r.CPUMax.Value {
		r.CPUMax = Extreme{s.CPUPct, s.Time}
	}
	if s.CPUPct < r.CPUMin.Value {
		r.CPUMin = Extreme{s.CPUPct, s.Time}
	}
	if float64(s.RSS) > r.MemMax.Value {
		r.MemMax = Extreme{float64(s.RSS), s.Time}
	}
	if float64(s.RSS) < r.MemMin.Value {
		r.MemMin = Extreme{float64(s.RSS), s.Time}
	}
}

// CPUAvg returns the mean CPU%% over all recorded samples.
func (r *Recorder) CPUAvg() float64 {
	if r.Samples == 0 {
		return 0
	}
	return r.cpuSum / float64(r.Samples)
}

// Annotate attaches a note to a sample span.
func (r *Recorder) Annotate(a Annotation) {
	if a.End.Before(a.Start) {
		a.Start, a.End = a.End, a.Start
	}
	r.Annotations = append(r.Annotations, a)
}

// SpanMax returns the maximum CPU%% and RSS among history samples within
// [start, end]; ok is false when no sample of the span is still in history.
func (r *Recorder) SpanMax(start, end time.Time) (cpu, rss float64, ok bool) {
	for _, s := range r.History {
		if s.Time.Before(start) || s.Time.After(end) {
			continue
		}
		ok = true
		cpu = max(cpu, s.CPUPct)
		rss = max(rss, float64(s.RSS))
	}
	return cpu, rss, ok
}

// MemAvg returns the mean RSS in bytes over all recorded samples.
func (r *Recorder) MemAvg() float64 {
	if r.Samples == 0 {
		return 0
	}
	return r.memSum / float64(r.Samples)
}
