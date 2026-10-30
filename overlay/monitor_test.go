package main

import "testing"

func TestParseStats(t *testing.T) {
	out := `cpu  100 0 50 800 50 0 0 0 0 0
MemTotal:       16000000 kB
MemAvailable:    4000000 kB
NET 5000 2500
UP 90061.55
/dev/sda1 100000000 30000000 70000000 30% /
USER ubuntu
`
	raw, st := parseStats(out)
	if raw.cpuTotal != 1000 || raw.cpuIdle != 850 {
		t.Fatalf("cpu: %+v", raw)
	}
	if raw.netRx != 5000 || raw.netTx != 2500 {
		t.Fatalf("net: %+v", raw)
	}
	if st.UptimeSec != 90061 || st.User != "ubuntu" || st.DiskPct != 30 {
		t.Fatalf("stats: %+v", st)
	}
	if st.MemTotal < 15 || st.MemTotal > 16 || st.MemUsed < 11 || st.MemUsed > 12 {
		t.Fatalf("mem: %+v", st)
	}
}

func TestParseStatsPartial(t *testing.T) {
	_, st := parseStats("USER root\n")
	if st.User != "root" || st.MemTotal != 0 {
		t.Fatalf("%+v", st)
	}
}
