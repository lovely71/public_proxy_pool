package ui

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

type systemMetrics struct {
	mu         sync.Mutex
	prevCPU    cpuSample
	hasPrevCPU bool
	lastCPU    float64
}

type cpuSample struct {
	total uint64
	idle  uint64
}

type memSnapshot struct {
	total uint64
	used  uint64
}

type systemSnapshot struct {
	cpuReady bool
	cpuUsage float64
	memory   memSnapshot
	swap     memSnapshot
}

func newSystemMetrics() *systemMetrics {
	m := &systemMetrics{}
	if s, err := readCPUSample(); err == nil {
		m.prevCPU = s
		m.hasPrevCPU = true
	}
	return m
}

func (m *systemMetrics) Snapshot() systemSnapshot {
	var snap systemSnapshot

	mem, swap, err := readMemorySnapshots()
	if err == nil {
		snap.memory = mem
		snap.swap = swap
	}

	sample, err := readCPUSample()
	if err != nil {
		return snap
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.hasPrevCPU {
		deltaTotal := sample.total - m.prevCPU.total
		deltaIdle := sample.idle - m.prevCPU.idle
		if deltaTotal > 0 {
			snap.cpuUsage = (1 - float64(deltaIdle)/float64(deltaTotal)) * 100
			snap.cpuReady = true
			m.lastCPU = snap.cpuUsage
		}
	}
	m.prevCPU = sample
	m.hasPrevCPU = true

	if !snap.cpuReady && m.lastCPU > 0 {
		snap.cpuUsage = m.lastCPU
		snap.cpuReady = true
	}

	return snap
}

func readCPUSample() (cpuSample, error) {
	if runtime.GOOS != "linux" {
		return cpuSample{}, fmt.Errorf("unsupported os: %s", runtime.GOOS)
	}

	f, err := os.Open("/proc/stat")
	if err != nil {
		return cpuSample{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return cpuSample{}, fmt.Errorf("unexpected cpu stat format")
		}

		var total uint64
		var values []uint64
		for _, raw := range fields[1:] {
			v, err := strconv.ParseUint(raw, 10, 64)
			if err != nil {
				return cpuSample{}, err
			}
			values = append(values, v)
			total += v
		}

		idle := values[3]
		if len(values) > 4 {
			idle += values[4]
		}
		return cpuSample{total: total, idle: idle}, nil
	}
	if err := scanner.Err(); err != nil {
		return cpuSample{}, err
	}
	return cpuSample{}, fmt.Errorf("cpu line not found")
}

func readMemorySnapshots() (memSnapshot, memSnapshot, error) {
	if runtime.GOOS != "linux" {
		return memSnapshot{}, memSnapshot{}, fmt.Errorf("unsupported os: %s", runtime.GOOS)
	}

	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return memSnapshot{}, memSnapshot{}, err
	}
	defer f.Close()

	values := map[string]uint64{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		values[key] = v * 1024
	}
	if err := scanner.Err(); err != nil {
		return memSnapshot{}, memSnapshot{}, err
	}

	memTotal := values["MemTotal"]
	memAvail := values["MemAvailable"]
	if memAvail == 0 {
		memAvail = values["MemFree"]
	}

	swapTotal := values["SwapTotal"]
	swapFree := values["SwapFree"]

	memUsed := uint64(0)
	if memTotal > memAvail {
		memUsed = memTotal - memAvail
	}
	swapUsed := uint64(0)
	if swapTotal > swapFree {
		swapUsed = swapTotal - swapFree
	}

	return memSnapshot{total: memTotal, used: memUsed}, memSnapshot{total: swapTotal, used: swapUsed}, nil
}

func formatPercent(v float64) string {
	if v < 0 {
		return "未读取到"
	}
	return fmt.Sprintf("%.1f%%", v)
}

func formatMemoryUsage(s memSnapshot, zero string) string {
	if s.total == 0 {
		return zero
	}
	return fmt.Sprintf("%s / %s (%.0f%%)", formatBytesIEC(s.used), formatBytesIEC(s.total), percentOf(s.used, s.total))
}

func percentOf(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(used) * 100 / float64(total)
}

func formatBytesIEC(v uint64) string {
	const unit = 1024
	if v < unit {
		return fmt.Sprintf("%d B", v)
	}

	div := uint64(unit)
	exp := 0
	for n := v / unit; n >= unit && exp < 5; n /= unit {
		div *= unit
		exp++
	}
	suffixes := []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}
	return fmt.Sprintf("%.1f %s", float64(v)/float64(div), suffixes[exp])
}
