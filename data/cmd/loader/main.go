// Comando principal del cargador concurrente de crímenes de Chicago.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"crimes-loader/data/internal/loader"
	"crimes-loader/data/internal/pipeline"
)

func main() {
	// ── Flags de línea de comandos ────────────────────────────────────────────
	fileFlag := flag.String("file", "data/crimes.csv", "Ruta al CSV del dataset Chicago Crimes")
	workersFlag := flag.Int("workers", runtime.NumCPU(), "Número de goroutines trabajadoras")
	chunkFlag := flag.Int("chunk", 5000, "Líneas CSV por bloque de trabajo")
	outputFlag := flag.String("output", "output", "Directorio de salida")
	benchFlag := flag.Bool("benchmark", false, "Ejecuta comparativa secuencial vs concurrente")
	verboseFlag := flag.Bool("verbose", true, "Imprime progreso en tiempo real")
	flag.Parse()

	fmt.Println("╔════════════════════════════════════════════════════╗")
	fmt.Println("║  CC65 — Cargador Concurrente de Datos              ║")
	fmt.Println("║  Dataset: Chicago Crimes 2001-Present              ║")
	fmt.Println("╚════════════════════════════════════════════════════╝")
	fmt.Printf("  CPU cores disponibles: %d\n", runtime.NumCPU())
	fmt.Printf("  Goroutines configuradas: %d\n", *workersFlag)

	// Verificar que el archivo existe
	if _, err := os.Stat(*fileFlag); os.IsNotExist(err) {
		log.Printf("  Archivo no encontrado: %s", *fileFlag)
		log.Println("   Generando dataset sintético de prueba (100 000 registros)...")
		if err := generateSyntheticCSV(*fileFlag, 100_000); err != nil {
			log.Fatalf("Error generando dataset de prueba: %v", err)
		}
		log.Printf("   Dataset sintético creado en: %s\n", *fileFlag)
	}

	cfg := loader.Config{
		NumWorkers: *workersFlag,
		ChunkSize:  *chunkFlag,
		BufferSize: *workersFlag * 4,
		Verbose:    *verboseFlag,
	}

	if *benchFlag {
		runBenchmark(*fileFlag, cfg)
		return
	}

	// ── Pipeline normal ──────────────────────────────────────────────────────
	p := pipeline.New(cfg)
	if err := p.Run(*fileFlag, *outputFlag); err != nil {
		log.Fatalf(" Error en el pipeline: %v", err)
	}
}

// runBenchmark compara el rendimiento entre 1 worker (pseudo-secuencial)
// y N workers (concurrente) para demostrar el speedup obtenido.
func runBenchmark(csvPath string, cfg loader.Config) {
	fmt.Println("\n  MODO BENCHMARK: Comparativa secuencial vs. concurrente")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	workerCounts := []int{1, 2, 4, cfg.NumWorkers}
	type result struct {
		workers int
		elapsed time.Duration
		records int64
	}
	results := make([]result, 0, len(workerCounts))

	for _, nw := range workerCounts {
		c := cfg
		c.NumWorkers = nw
		c.Verbose = false
		l := loader.New(c)

		fmt.Printf("  Ejecutando con %2d worker(s)... ", nw)
		start := time.Now()
		records, stats, err := l.LoadFile(csvPath)
		elapsed := time.Since(start)

		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			continue
		}
		_ = records
		results = append(results, result{workers: nw, elapsed: elapsed, records: stats.ValidRecords})
		fmt.Printf("✓ %d registros en %v\n", stats.ValidRecords, elapsed.Round(time.Millisecond))
	}

	if len(results) == 0 {
		return
	}

	fmt.Println("\n  Resultados de speedup:")
	fmt.Println("  ┌─────────┬─────────────┬──────────┐")
	fmt.Println("  │ Workers │ Tiempo      │ Speedup  │")
	fmt.Println("  ├─────────┼─────────────┼──────────┤")
	baseline := results[0].elapsed
	for _, r := range results {
		speedup := float64(baseline) / float64(r.elapsed)
		fmt.Printf("  │ %7d │ %11v │ %7.2fx │\n", r.workers, r.elapsed.Round(time.Millisecond), speedup)
	}
	fmt.Println("  └─────────┴─────────────┴──────────┘")
}
