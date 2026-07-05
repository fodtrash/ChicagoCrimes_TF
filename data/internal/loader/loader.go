// Package loader implementa la carga concurrente de archivos CSV masivos.
package loader

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"crimes-loader/data/internal/cleaner"
	"crimes-loader/data/internal/models"
)

// Config parametriza el comportamiento del cargador concurrente.
type Config struct {
	// NumWorkers es el número de goroutines trabajadoras (recomendado: runtime.NumCPU())
	NumWorkers int
	// ChunkSize es el número de líneas CSV por bloque de trabajo
	ChunkSize int
	// BufferSize es la capacidad de los canales internos
	BufferSize int
	// Verbose activa mensajes de progreso en stdout
	Verbose bool
}

// DefaultConfig retorna una configuración razonable para la mayoría de máquinas.
func DefaultConfig() Config {
	return Config{
		NumWorkers: 8,
		ChunkSize:  5000,
		BufferSize: 100,
		Verbose:    true,
	}
}

// chunk es la unidad de trabajo que se envía a cada worker.
type chunk struct {
	lines     []string // líneas CSV sin procesar
	startLine int      // número de línea de inicio (para mensajes de error)
}

// workerResult es el resultado que produce cada worker al procesar un chunk.
type workerResult struct {
	records []*models.CleanRecord
	errors  []*models.ParseError
}

// ConcurrentLoader es el cargador concurrente principal.
type ConcurrentLoader struct {
	cfg Config
}

// New crea un nuevo ConcurrentLoader con la configuración dada.
func New(cfg Config) *ConcurrentLoader {
	if cfg.NumWorkers <= 0 {
		cfg.NumWorkers = 4
	}
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 5000
	}
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 50
	}
	return &ConcurrentLoader{cfg: cfg}
}

func (l *ConcurrentLoader) LoadFile(path string) ([]*models.CleanRecord, *models.LoadStats, error) {
	start := time.Now()

	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("no se puede abrir el archivo: %w", err)
	}
	defer f.Close()

	// Canales del pipeline
	chunkCh := make(chan chunk, l.cfg.BufferSize)
	resultCh := make(chan workerResult, l.cfg.BufferSize)

	// Contadores atómicos para el progreso
	var linesRead atomic.Int64
	var chunksDispatched atomic.Int64

	// ── Paso 1: goroutine lectora de archivo ──────────────────────────────────
	go func() {
		defer close(chunkCh)
		l.fileReader(f, chunkCh, &linesRead, &chunksDispatched)
	}()

	// ── Paso 2: pool de N workers goroutines ──────────────────────────────────
	var wg sync.WaitGroup
	for i := 0; i < l.cfg.NumWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			l.worker(workerID, chunkCh, resultCh)
		}(i)
	}

	// Goroutine que cierra resultCh cuando todos los workers terminan
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// ── Paso 3: agregador ─────────────────────────────────────────────────────
	records, stats := l.aggregator(resultCh, l.cfg.NumWorkers)

	elapsed := time.Since(start)
	stats.TotalLines = linesRead.Load()
	stats.WorkersUsed = l.cfg.NumWorkers
	stats.ChunkSize = l.cfg.ChunkSize
	stats.ElapsedMs = elapsed.Milliseconds()
	if elapsed.Seconds() > 0 {
		stats.RecordsPerSec = float64(stats.ValidRecords) / elapsed.Seconds()
	}

	if l.cfg.Verbose {
		printStats(stats)
	}

	return records, stats, nil
}

func (l *ConcurrentLoader) fileReader(
	f *os.File,
	chunkCh chan<- chunk,
	linesRead *atomic.Int64,
	chunksDispatched *atomic.Int64,
) {
	reader := bufio.NewReaderSize(f, 4*1024*1024) // buffer de 4 MB
	lineNum := 0
	firstLine := true
	currentChunk := make([]string, 0, l.cfg.ChunkSize)
	chunkStartLine := 1

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			break
		}

		line = strings.TrimRight(line, "\r\n")
		lineNum++

		// Detectar y saltar encabezado
		if firstLine {
			firstLine = false
			// Parsear como CSV para detectar si es encabezado
			r := csv.NewReader(strings.NewReader(line))
			fields, csvErr := r.Read()
			if csvErr == nil && cleaner.IsHeaderRow(fields) {
				if err == io.EOF {
					break
				}
				continue
			}
		}

		if line != "" {
			currentChunk = append(currentChunk, line)
			linesRead.Add(1)
		}

		// Cuando el chunk alcanza el tamaño configurado, lo envía
		if len(currentChunk) >= l.cfg.ChunkSize {
			chunkCh <- chunk{lines: currentChunk, startLine: chunkStartLine}
			chunksDispatched.Add(1)
			chunkStartLine = lineNum + 1
			currentChunk = make([]string, 0, l.cfg.ChunkSize)
		}

		if err == io.EOF {
			break
		}
	}

	// Enviar el último chunk parcial
	if len(currentChunk) > 0 {
		chunkCh <- chunk{lines: currentChunk, startLine: chunkStartLine}
		chunksDispatched.Add(1)
	}
}

func (l *ConcurrentLoader) worker(id int, chunkCh <-chan chunk, resultCh chan<- workerResult) {
	for ch := range chunkCh {
		result := workerResult{
			records: make([]*models.CleanRecord, 0, len(ch.lines)),
			errors:  make([]*models.ParseError, 0),
		}

		for i, line := range ch.lines {
			lineNum := ch.startLine + i

			// Parsear CSV respetando campos entre comillas
			r := csv.NewReader(strings.NewReader(line))
			r.LazyQuotes = true
			r.TrimLeadingSpace = true
			fields, err := r.Read()
			if err != nil {
				result.errors = append(result.errors, &models.ParseError{
					Line:   lineNum,
					Field:  "csv",
					Reason: "error de parseo CSV: " + err.Error(),
				})
				continue
			}

			record, parseErr := cleaner.ParseAndClean(fields, lineNum)
			if parseErr != nil {
				result.errors = append(result.errors, parseErr)
				continue
			}

			result.records = append(result.records, record)
		}

		resultCh <- result
	}
}

func (l *ConcurrentLoader) aggregator(resultCh <-chan workerResult, numWorkers int) ([]*models.CleanRecord, *models.LoadStats) {
	// Pre-asignamos capacidad estimada (ajústala según tu dataset)
	allRecords := make([]*models.CleanRecord, 0, 1_000_000)
	stats := &models.LoadStats{
		ErrorSummary: make(map[string]int),
	}

	progressTicker := time.NewTicker(5 * time.Second)
	defer progressTicker.Stop()

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-progressTicker.C:
				if l.cfg.Verbose {
					fmt.Printf("  [progreso] registros válidos hasta ahora: %d, errores: %d\n",
						stats.ValidRecords, stats.InvalidRecords)
				}
			case <-done:
				return
			}
		}
	}()

	for result := range resultCh {
		allRecords = append(allRecords, result.records...)
		stats.ValidRecords += int64(len(result.records))

		for _, e := range result.errors {
			stats.InvalidRecords++
			stats.ErrorSummary[e.Field]++
		}
	}

	close(done)
	return allRecords, stats
}

// printStats imprime un resumen de las estadísticas de carga.
func printStats(s *models.LoadStats) {
	fmt.Println("\n╔══════════════════════════════════════════════════════╗")
	fmt.Println("║         RESUMEN DE CARGA CONCURRENTE                ║")
	fmt.Println("╠══════════════════════════════════════════════════════╣")
	fmt.Printf("║  Líneas leídas     : %10d                       ║\n", s.TotalLines)
	fmt.Printf("║  Registros válidos : %10d                       ║\n", s.ValidRecords)
	fmt.Printf("║  Registros inválidos:%9d                       ║\n", s.InvalidRecords)
	fmt.Printf("║  Workers usados    : %10d                       ║\n", s.WorkersUsed)
	fmt.Printf("║  Tamaño de chunk   : %10d líneas                ║\n", s.ChunkSize)
	fmt.Printf("║  Tiempo total      : %10d ms                    ║\n", s.ElapsedMs)
	fmt.Printf("║  Registros/segundo : %10.0f                       ║\n", s.RecordsPerSec)
	fmt.Println("╠══════════════════════════════════════════════════════╣")
	if len(s.ErrorSummary) > 0 {
		fmt.Println("║  Errores por campo:                                  ║")
		for field, count := range s.ErrorSummary {
			fmt.Printf("║    %-20s : %6d                       ║\n", field, count)
		}
	} else {
		fmt.Println("║  Sin errores de parseo                               ║")
	}
	fmt.Println("╚══════════════════════════════════════════════════════╝")
}
