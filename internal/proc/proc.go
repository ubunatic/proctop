// Package proc reads process information from /proc: per-process CPU tick
// and RSS counters, and process-tree discovery by PID or by name.
package proc

import (
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Stat holds one process's counters read from /proc/<pid>/stat.
type Stat struct {
	PID   int
	Comm  string
	PPID  int
	Ticks uint64 // utime + stime, in clock ticks
	RSS   int64  // resident set size in bytes
}

// ReadAll scans /proc and returns the stat of every readable process,
// keyed by PID. Processes that vanish mid-scan are skipped.
func ReadAll() (map[int]Stat, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("proc: %w", err)
	}
	pageSize := int64(os.Getpagesize())
	all := make(map[int]Stat, len(entries))
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue // vanished or unreadable
		}
		st, err := parseStat(pid, string(data), pageSize)
		if err != nil {
			continue
		}
		all[pid] = st
	}
	return all, nil
}

// parseStat parses one /proc/<pid>/stat line. The comm field is enclosed in
// parentheses and may itself contain spaces and parentheses, so fields are
// split only after the last ')'.
func parseStat(pid int, data string, pageSize int64) (Stat, error) {
	open := strings.IndexByte(data, '(')
	close := strings.LastIndexByte(data, ')')
	if open < 0 || close < open {
		return Stat{}, fmt.Errorf("proc: malformed stat for pid %d", pid)
	}
	comm := data[open+1 : close]
	fields := strings.Fields(data[close+1:])
	// fields[0] is state (field 3 in proc(5)); man-page field N maps to fields[N-3].
	const (
		fPPID  = 4 - 3
		fUtime = 14 - 3
		fStime = 15 - 3
		fRSS   = 24 - 3
	)
	if len(fields) <= fRSS {
		return Stat{}, fmt.Errorf("proc: short stat for pid %d", pid)
	}
	ppid, err := strconv.Atoi(fields[fPPID])
	if err != nil {
		return Stat{}, fmt.Errorf("proc: ppid of pid %d: %w", pid, err)
	}
	utime, err := strconv.ParseUint(fields[fUtime], 10, 64)
	if err != nil {
		return Stat{}, fmt.Errorf("proc: utime of pid %d: %w", pid, err)
	}
	stime, err := strconv.ParseUint(fields[fStime], 10, 64)
	if err != nil {
		return Stat{}, fmt.Errorf("proc: stime of pid %d: %w", pid, err)
	}
	rssPages, err := strconv.ParseInt(fields[fRSS], 10, 64)
	if err != nil {
		return Stat{}, fmt.Errorf("proc: rss of pid %d: %w", pid, err)
	}
	return Stat{
		PID:   pid,
		Comm:  comm,
		PPID:  ppid,
		Ticks: utime + stime,
		RSS:   rssPages * pageSize,
	}, nil
}

// FindRoot resolves a watch target to a root PID. A numeric target is used
// as a PID directly; otherwise the target is matched case-insensitively
// against process names. Among the topmost matching processes (whose parent
// does not match) the one with the largest process tree wins, so launcher
// stubs and forks lose against the real main process; ties go to the lowest
// PID.
func FindRoot(all map[int]Stat, target string) (int, error) {
	if pid, err := strconv.Atoi(target); err == nil {
		if _, ok := all[pid]; !ok {
			return 0, fmt.Errorf("proc: no process with pid %d", pid)
		}
		return pid, nil
	}
	needle := strings.ToLower(target)
	matches := make(map[int]bool)
	for pid, st := range all {
		if strings.Contains(strings.ToLower(st.Comm), needle) {
			matches[pid] = true
		}
	}
	if len(matches) == 0 {
		return 0, fmt.Errorf("proc: no process matching %q", target)
	}
	var roots []int
	for pid := range matches {
		if !matches[all[pid].PPID] {
			roots = append(roots, pid)
		}
	}
	sort.Ints(roots)
	best, bestSize := roots[0], -1
	for _, pid := range roots {
		if size := len(Tree(all, pid)); size > bestSize {
			best, bestSize = pid, size
		}
	}
	return best, nil
}

// Tree returns the stats of root and all its descendants, ordered by PID.
// If root is not present in all, the returned slice is empty.
func Tree(all map[int]Stat, root int) []Stat {
	children := make(map[int][]int, len(all))
	for pid, st := range all {
		children[st.PPID] = append(children[st.PPID], pid)
	}
	if _, ok := all[root]; !ok {
		return nil
	}
	var pids []int
	queue := []int{root}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		pids = append(pids, pid)
		queue = append(queue, children[pid]...)
	}
	sort.Ints(pids)
	tree := make([]Stat, 0, len(pids))
	for _, pid := range pids {
		tree = append(tree, all[pid])
	}
	return tree
}

// ClockTicks returns the kernel's USER_HZ (ticks per second) from the ELF
// auxiliary vector (AT_CLKTCK), falling back to the conventional 100.
func ClockTicks() int64 {
	const atClktck = 17
	data, err := os.ReadFile("/proc/self/auxv")
	if err != nil {
		return 100
	}
	for i := 0; i+16 <= len(data); i += 16 {
		key := binary.NativeEndian.Uint64(data[i:])
		if key == atClktck {
			return int64(binary.NativeEndian.Uint64(data[i+8:]))
		}
	}
	return 100
}
