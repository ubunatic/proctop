package proc

import (
	"os"
	"testing"
)

const statLine = `1234 (Web Content (x)) S 1000 1234 1000 0 -1 4194560 ` +
	`100 0 0 0 500 250 0 0 20 0 30 0 12345 1073741824 4096 ` +
	`18446744073709551615 1 1 0 0 0 0 0 0 0 0 0 0 17 3 0 0 0 0 0`

func TestParseStat(t *testing.T) {
	st, err := parseStat(1234, statLine, 4096)
	if err != nil {
		t.Fatalf("parseStat: %v", err)
	}
	if st.Comm != "Web Content (x)" {
		t.Errorf("comm = %q, want %q", st.Comm, "Web Content (x)")
	}
	if st.PPID != 1000 {
		t.Errorf("ppid = %d, want 1000", st.PPID)
	}
	if st.Ticks != 750 {
		t.Errorf("ticks = %d, want 750 (utime 500 + stime 250)", st.Ticks)
	}
	if st.RSS != 4096*4096 {
		t.Errorf("rss = %d, want %d", st.RSS, 4096*4096)
	}
}

func TestParseStatMalformed(t *testing.T) {
	for _, data := range []string{"", "1234 no-parens S", "1234 (x) S 1"} {
		if _, err := parseStat(1, data, 4096); err == nil {
			t.Errorf("parseStat(%q) should fail", data)
		}
	}
}

func mkStat(pid, ppid int, comm string) Stat {
	return Stat{PID: pid, PPID: ppid, Comm: comm}
}

func testProcs() map[int]Stat {
	return map[int]Stat{
		1:   mkStat(1, 0, "systemd"),
		50:  mkStat(50, 1, "firefox"), // childless launcher stub, must not win
		100: mkStat(100, 1, "firefox"),
		101: mkStat(101, 100, "firefox"),
		102: mkStat(102, 100, "Isolated Web Co"),
		103: mkStat(103, 102, "helper"),
		200: mkStat(200, 1, "bash"),
	}
}

func TestFindRootByName(t *testing.T) {
	pid, err := FindRoot(testProcs(), "firefox")
	if err != nil {
		t.Fatalf("FindRoot: %v", err)
	}
	if pid != 100 {
		t.Errorf("root = %d, want 100 (largest tree beats lower-pid stub)", pid)
	}
}

func TestFindRootByPID(t *testing.T) {
	pid, err := FindRoot(testProcs(), "102")
	if err != nil {
		t.Fatalf("FindRoot: %v", err)
	}
	if pid != 102 {
		t.Errorf("root = %d, want 102", pid)
	}
	if _, err := FindRoot(testProcs(), "999"); err == nil {
		t.Error("unknown pid should fail")
	}
	if _, err := FindRoot(testProcs(), "no-such-proc"); err == nil {
		t.Error("unknown name should fail")
	}
}

func TestTree(t *testing.T) {
	tree := Tree(testProcs(), 100)
	want := []int{100, 101, 102, 103}
	if len(tree) != len(want) {
		t.Fatalf("tree size = %d, want %d", len(tree), len(want))
	}
	for i, st := range tree {
		if st.PID != want[i] {
			t.Errorf("tree[%d] = %d, want %d", i, st.PID, want[i])
		}
	}
	if got := Tree(testProcs(), 999); len(got) != 0 {
		t.Errorf("tree of missing root should be empty, got %v", got)
	}
}

func TestReadAllIncludesSelf(t *testing.T) {
	all, err := ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	self, ok := all[os.Getpid()]
	if !ok {
		t.Fatalf("own pid %d not found", os.Getpid())
	}
	if self.RSS <= 0 {
		t.Errorf("own rss = %d, want > 0", self.RSS)
	}
}

func TestClockTicks(t *testing.T) {
	if hz := ClockTicks(); hz < 1 || hz > 10000 {
		t.Errorf("ClockTicks = %d, implausible", hz)
	}
}
