// Package pipeline coordina la carga, limpieza y persistencia de resultados.
// Actúa como orquestador del flujo completo: CSV → CleanRecords → JSON/Stats.
package pipeline

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"crimes-loader/data/internal/loader"
	"crimes-loader/data/internal/models"
)

// Pipeline orquesta el proceso de carga de datos concurrente.
type Pipeline struct {
	cfg loader.Config
}

// New crea un nuevo Pipeline con la configuración dada.
func New(cfg loader.Config) *Pipeline {
	return &Pipeline{cfg: cfg}
}

// Run ejecuta el pipeline completo sobre el archivo CSV dado y escribe
// los resultados en el directorio outputDir.
func (p *Pipeline) Run(csvPath, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("no se puede crear directorio de salida: %w", err)
	}

	fmt.Printf("\nIniciando carga concurrente del dataset de Chicago Crimes\n")
	fmt.Printf("   Archivo    : %s\n", csvPath)
	fmt.Printf("   Workers    : %d goroutines\n", p.cfg.NumWorkers)
	fmt.Printf("   Chunk size : %d líneas por bloque\n", p.cfg.ChunkSize)
	fmt.Printf("   Inicio     : %s\n\n", time.Now().Format("15:04:05"))

	// ── Carga concurrente ───────────────────────────────────────────────────
	l := loader.New(p.cfg)
	records, stats, err := l.LoadFile(csvPath)
	if err != nil {
		return fmt.Errorf("error en la carga: %w", err)
	}

	// ── Guardar estadísticas de carga ───────────────────────────────────────
	statsPath := filepath.Join(outputDir, "load_stats.json")
	if err := writeJSON(statsPath, stats); err != nil {
		return err
	}
	fmt.Printf("\n Estadísticas guardadas en: %s\n", statsPath)

	// ── Análisis de zonas y franjas horarias ─────────────────────────────────
	analysis := analyzeRiskZones(records)
	analysisPath := filepath.Join(outputDir, "risk_analysis.json")
	if err := writeJSON(analysisPath, analysis); err != nil {
		return err
	}
	fmt.Printf("  Análisis de riesgo guardado en: %s\n", analysisPath)

	sampleSize := 100_000
	if v := os.Getenv("SAMPLE_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			sampleSize = n
		}
	}
	if len(records) < sampleSize {
		sampleSize = len(records)
	}
	rng := rand.New(rand.NewSource(42))
	rng.Shuffle(len(records), func(i, j int) { records[i], records[j] = records[j], records[i] })
	samplePath := filepath.Join(outputDir, "clean_records_sample.json")
	if err := writeJSON(samplePath, records[:sampleSize]); err != nil {
		return err
	}
	fmt.Printf(" Muestra aleatoria de %d registros limpios en: %s\n", sampleSize, samplePath)

	fmt.Printf("\n Pipeline completado exitosamente.\n")
	printRiskSummary(analysis)
	return nil
}

// ── Análisis de riesgo ─────────────────────────────────────────────────────────

// RiskAnalysis contiene el resultado del análisis de zonas y franjas horarias.
type RiskAnalysis struct {
	GeneratedAt       time.Time      `json:"generated_at"`
	TotalRecords      int            `json:"total_records"`
	CrimesByHour      map[int]int    `json:"crimes_by_hour"`
	CrimesByDistrict  map[int]int    `json:"crimes_by_district"`
	CrimesByDayOfWeek map[int]int    `json:"crimes_by_day_of_week"`
	CrimesByType      map[string]int `json:"crimes_by_type"`
	TopRiskHours      []HourRisk     `json:"top_risk_hours"`
	TopRiskDistricts  []DistrictRisk `json:"top_risk_districts"`
	HeatmapData       []HeatmapCell  `json:"heatmap_data"`
}

// HourRisk representa la peligrosidad de una franja horaria.
type HourRisk struct {
	Hour           int     `json:"hour"`
	CrimeCount     int     `json:"crime_count"`
	RiskScore      float64 `json:"risk_score"` // normalizado 0-1
	MostCommonType string  `json:"most_common_type"`
}

// DistrictRisk representa el nivel de crimen de un distrito.
type DistrictRisk struct {
	District   int     `json:"district"`
	CrimeCount int     `json:"crime_count"`
	RiskScore  float64 `json:"risk_score"`
	ArrestRate float64 `json:"arrest_rate"`
}

// HeatmapCell es una celda del mapa de calor hora × distrito.
type HeatmapCell struct {
	Hour     int `json:"hour"`
	District int `json:"district"`
	Count    int `json:"count"`
}

// analyzeRiskZones computa el análisis de riesgo sobre los registros limpios.
// Esta función responde la pregunta de impacto social del proyecto.
func analyzeRiskZones(records []*models.CleanRecord) *RiskAnalysis {
	analysis := &RiskAnalysis{
		GeneratedAt:       time.Now(),
		TotalRecords:      len(records),
		CrimesByHour:      make(map[int]int),
		CrimesByDistrict:  make(map[int]int),
		CrimesByDayOfWeek: make(map[int]int),
		CrimesByType:      make(map[string]int),
	}

	// Conteos por distrito×hora para el heatmap
	heatmap := make(map[[2]int]int)
	// Arrestos por distrito
	arrestsByDistrict := make(map[int]int)
	// Tipos de crimen por hora
	crimeTypeByHour := make(map[int]map[string]int)

	for _, r := range records {
		analysis.CrimesByHour[r.Hour]++
		analysis.CrimesByDistrict[r.District]++
		analysis.CrimesByDayOfWeek[r.DayOfWeek]++
		analysis.CrimesByType[r.PrimaryType]++

		key := [2]int{r.Hour, r.District}
		heatmap[key]++

		if r.Arrest {
			arrestsByDistrict[r.District]++
		}

		if _, ok := crimeTypeByHour[r.Hour]; !ok {
			crimeTypeByHour[r.Hour] = make(map[string]int)
		}
		crimeTypeByHour[r.Hour][r.PrimaryType]++
	}

	// ── Top 5 franjas horarias de mayor riesgo ───────────────────────────────
	maxHour := 0
	for _, v := range analysis.CrimesByHour {
		if v > maxHour {
			maxHour = v
		}
	}
	hourRisks := make([]HourRisk, 0, 24)
	for h := 0; h < 24; h++ {
		count := analysis.CrimesByHour[h]
		score := 0.0
		if maxHour > 0 {
			score = math.Round(float64(count)/float64(maxHour)*100) / 100
		}
		mostCommon := ""
		if types, ok := crimeTypeByHour[h]; ok {
			mostCommon = maxKey(types)
		}
		hourRisks = append(hourRisks, HourRisk{
			Hour:           h,
			CrimeCount:     count,
			RiskScore:      score,
			MostCommonType: mostCommon,
		})
	}
	sort.Slice(hourRisks, func(i, j int) bool {
		return hourRisks[i].CrimeCount > hourRisks[j].CrimeCount
	})
	top := 5
	if len(hourRisks) < top {
		top = len(hourRisks)
	}
	analysis.TopRiskHours = hourRisks[:top]

	// ── Top 5 distritos de mayor riesgo ─────────────────────────────────────
	maxDistrict := 0
	for _, v := range analysis.CrimesByDistrict {
		if v > maxDistrict {
			maxDistrict = v
		}
	}
	districtRisks := make([]DistrictRisk, 0, len(analysis.CrimesByDistrict))
	for d, count := range analysis.CrimesByDistrict {
		if d == 0 {
			continue // distrito 0 = desconocido
		}
		score := 0.0
		if maxDistrict > 0 {
			score = math.Round(float64(count)/float64(maxDistrict)*100) / 100
		}
		arrestRate := 0.0
		if count > 0 {
			arrestRate = math.Round(float64(arrestsByDistrict[d])/float64(count)*100) / 100
		}
		districtRisks = append(districtRisks, DistrictRisk{
			District:   d,
			CrimeCount: count,
			RiskScore:  score,
			ArrestRate: arrestRate,
		})
	}
	sort.Slice(districtRisks, func(i, j int) bool {
		return districtRisks[i].CrimeCount > districtRisks[j].CrimeCount
	})
	top = 5
	if len(districtRisks) < top {
		top = len(districtRisks)
	}
	analysis.TopRiskDistricts = districtRisks[:top]

	// ── Heatmap hora × distrito ──────────────────────────────────────────────
	cells := make([]HeatmapCell, 0, len(heatmap))
	for k, count := range heatmap {
		cells = append(cells, HeatmapCell{Hour: k[0], District: k[1], Count: count})
	}
	sort.Slice(cells, func(i, j int) bool {
		return cells[i].Count > cells[j].Count
	})
	// Guardamos solo las 200 celdas más activas para limitar el JSON
	if len(cells) > 200 {
		cells = cells[:200]
	}
	analysis.HeatmapData = cells

	return analysis
}

// ── helpers ────────────────────────────────────────────────────────────────────

func writeJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("no se puede crear %s: %w", path, err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func maxKey(m map[string]int) string {
	best, bestVal := "", 0
	for k, v := range m {
		if v > bestVal {
			bestVal = v
			best = k
		}
	}
	return best
}

func printRiskSummary(a *RiskAnalysis) {
	fmt.Println("\n╔══════════════════════════════════════════════════════╗")
	fmt.Println("║     ANÁLISIS DE RIESGO — CHICAGO CRIMES              ║")
	fmt.Println("╠══════════════════════════════════════════════════════╣")
	fmt.Println("║  Top 5 Franjas Horarias de Mayor Riesgo              ║")
	for i, h := range a.TopRiskHours {
		fmt.Printf("║  %d. Hora %02d:00 — %6d crímenes (score %.2f) %-8s║\n",
			i+1, h.Hour, h.CrimeCount, h.RiskScore, "")
	}
	fmt.Println("╠══════════════════════════════════════════════════════╣")
	fmt.Println("║  Top 5 Distritos de Mayor Riesgo                     ║")
	for i, d := range a.TopRiskDistricts {
		fmt.Printf("║  %d. Distrito %3d  — %6d crímenes (arrestos %.0f%%) %-2s║\n",
			i+1, d.District, d.CrimeCount, d.ArrestRate*100, "")
	}
	fmt.Println("╚══════════════════════════════════════════════════════╝")
}
