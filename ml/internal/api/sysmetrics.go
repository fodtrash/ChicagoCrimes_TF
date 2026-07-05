package api

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type sysMetrics struct {
	mu         sync.RWMutex
	systemPct  float64 // % de CPU de todo el sistema
	processPct float64 // % de CPU del proceso de la API

	prevIdle, prevTotal uint64 // acumuladores de /proc/stat
	prevProc            uint64 // utime+stime de /proc/self/stat
	prevAt              time.Time
}

// newSysMetrics arranca la goroutine de muestreo (cada 2 s).
func newSysMetrics() *sysMetrics {
	sm := &sysMetrics{prevAt: time.Now()}
	sm.sample() // primera lectura para inicializar acumuladores
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			sm.sample()
		}
	}()
	return sm
}

// sample calcula los porcentajes como deltas entre lecturas sucesivas.
func (sm *sysMetrics) sample() {
	idle, total, okSys := readProcStat()
	proc, okProc := readSelfStat()
	now := time.Now()

	sm.mu.Lock()
	defer sm.mu.Unlock()

	if okSys && sm.prevTotal > 0 && total > sm.prevTotal {
		dTotal := float64(total - sm.prevTotal)
		dIdle := float64(idle - sm.prevIdle)
		sm.systemPct = (1 - dIdle/dTotal) * 100
	}
	if okProc && sm.prevProc > 0 {
		// ticks del proceso / ticks de pared (100 Hz estándar en Linux)
		elapsedTicks := now.Sub(sm.prevAt).Seconds() * 100
		if elapsedTicks > 0 {
			sm.processPct = float64(proc-sm.prevProc) / elapsedTicks * 100
		}
	}
	sm.prevIdle, sm.prevTotal, sm.prevProc, sm.prevAt = idle, total, proc, now
}

// snapshot devuelve los valores actuales más memoria y goroutines.
func (sm *sysMetrics) snapshot() map[string]interface{} {
	sm.mu.RLock()
	sys, proc := sm.systemPct, sm.processPct
	sm.mu.RUnlock()

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	return map[string]interface{}{
		"cpu_system_percent":  round1(sys),
		"cpu_process_percent": round1(proc),
		// CPU del proceso de la API expresado como fracción de la
		// capacidad TOTAL de la máquina (todos los núcleos lógicos).
		"cpu_api_percent": round1(proc / float64(runtime.NumCPU())),
		"memory_mb":           round1(float64(ms.Alloc) / 1024 / 1024),
		"goroutines":          runtime.NumGoroutine(),
		"num_cpu":             runtime.NumCPU(),
	}
}

func round1(f float64) float64 {
	v, _ := strconv.ParseFloat(fmt.Sprintf("%.1f", f), 64)
	return v
}

// readProcStat lee la línea "cpu" agregada de /proc/stat y devuelve
// (ticks idle, ticks totales).
func readProcStat() (idle, total uint64, ok bool) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return 0, 0, false
	}
	fields := strings.Fields(sc.Text()) // cpu user nice system idle iowait ...
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, false
	}
	for i, fv := range fields[1:] {
		n, err := strconv.ParseUint(fv, 10, 64)
		if err != nil {
			continue
		}
		total += n
		if i == 3 || i == 4 { // idle + iowait
			idle += n
		}
	}
	return idle, total, true
}

// readSelfStat devuelve utime+stime (ticks de CPU) del proceso actual.
func readSelfStat() (uint64, bool) {
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0, false
	}
	// el nombre del proceso va entre paréntesis y puede contener espacios
	s := string(data)
	end := strings.LastIndex(s, ")")
	if end < 0 {
		return 0, false
	}
	fields := strings.Fields(s[end+1:])
	if len(fields) < 13 {
		return 0, false
	}
	utime, err1 := strconv.ParseUint(fields[11], 10, 64)
	stime, err2 := strconv.ParseUint(fields[12], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return utime + stime, true
}
