package main

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// Stats é o resumo do servidor mostrado na barra de status (monitoramento remoto).
type Stats struct {
	CPU       float64 `json:"cpu"`      // % de uso desde a última leitura
	MemUsed   float64 `json:"memUsed"`  // GB
	MemTotal  float64 `json:"memTotal"` // GB
	NetDown   float64 `json:"netDown"`  // bytes por segundo
	NetUp     float64 `json:"netUp"`    // bytes por segundo
	UptimeSec int64   `json:"uptimeSec"`
	User      string  `json:"user"`
	DiskPct   int     `json:"diskPct"` // uso do "/" em %
}

// rawStats guarda os contadores da leitura anterior para calcular as taxas.
type rawStats struct {
	at                time.Time
	cpuTotal, cpuIdle uint64
	netRx, netTx      uint64
}

// Script executado no servidor (stdin do "sh"): só lê /proc, df e whoami.
const statsScript = `head -n1 /proc/stat
grep -E '^(MemTotal|MemAvailable):' /proc/meminfo
awk 'NR>2{gsub(":"," ");if($1!="lo"){r+=$2;t+=$10}}END{print "NET",r+0,t+0}' /proc/net/dev
echo UP $(cut -d' ' -f1 /proc/uptime)
df -P / | tail -n1
echo USER $(whoami)
`

// Stats consulta o servidor da aba (Linux). Abre um canal SSH curto, sem tocar no terminal.
func (a *TermService) Stats(id string) (Stats, error) {
	t, err := a.term(id)
	if err != nil {
		return Stats{}, err
	}
	if t.kind != "ssh" {
		return Stats{}, errors.New("o monitoramento só funciona em sessões SSH")
	}
	client, err := t.sshClient()
	if err != nil {
		return Stats{}, err
	}
	sess, err := client.NewSession()
	if err != nil {
		return Stats{}, err
	}
	defer sess.Close()

	sess.Stdin = strings.NewReader(statsScript)
	type result struct {
		out []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := sess.Output("sh")
		ch <- result{out, err}
	}()
	var res result
	select {
	case res = <-ch:
	case <-time.After(8 * time.Second):
		sess.Close()
		return Stats{}, errors.New("o servidor demorou para responder")
	}
	if res.err != nil && len(res.out) == 0 {
		return Stats{}, res.err
	}

	cur, st := parseStats(string(res.out))
	t.statsMu.Lock()
	prev := t.prevStats
	t.prevStats = &cur
	t.statsMu.Unlock()

	if prev != nil {
		if dt := cur.at.Sub(prev.at).Seconds(); dt > 0 {
			if cur.cpuTotal > prev.cpuTotal {
				total := float64(cur.cpuTotal - prev.cpuTotal)
				idle := float64(cur.cpuIdle - prev.cpuIdle)
				st.CPU = clamp((total-idle)/total*100, 0, 100)
			}
			if cur.netRx >= prev.netRx {
				st.NetDown = float64(cur.netRx-prev.netRx) / dt
			}
			if cur.netTx >= prev.netTx {
				st.NetUp = float64(cur.netTx-prev.netTx) / dt
			}
		}
	}
	return st, nil
}

// parseStats lê a saída de statsScript. Linhas que faltam (sistema que não é
// Linux, por exemplo) simplesmente ficam zeradas.
func parseStats(out string) (rawStats, Stats) {
	raw := rawStats{at: time.Now()}
	var st Stats
	var memTotalKB, memAvailKB float64
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		switch {
		case f[0] == "cpu" && len(f) >= 5:
			var total, idle uint64
			for i, v := range f[1:] {
				n, _ := strconv.ParseUint(v, 10, 64)
				total += n
				if i == 3 || i == 4 { // idle + iowait
					idle += n
				}
			}
			raw.cpuTotal, raw.cpuIdle = total, idle
		case f[0] == "MemTotal:" && len(f) >= 2:
			memTotalKB, _ = strconv.ParseFloat(f[1], 64)
		case f[0] == "MemAvailable:" && len(f) >= 2:
			memAvailKB, _ = strconv.ParseFloat(f[1], 64)
		case f[0] == "NET" && len(f) >= 3:
			raw.netRx, _ = strconv.ParseUint(f[1], 10, 64)
			raw.netTx, _ = strconv.ParseUint(f[2], 10, 64)
		case f[0] == "UP" && len(f) >= 2:
			v, _ := strconv.ParseFloat(f[1], 64)
			st.UptimeSec = int64(v)
		case f[0] == "USER" && len(f) >= 2:
			st.User = f[1]
		case len(f) >= 5 && strings.HasSuffix(f[4], "%"):
			st.DiskPct, _ = strconv.Atoi(strings.TrimSuffix(f[4], "%"))
		}
	}
	const gb = 1024 * 1024
	st.MemTotal = memTotalKB / gb
	if memTotalKB > 0 {
		st.MemUsed = (memTotalKB - memAvailKB) / gb
	}
	return raw, st
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
