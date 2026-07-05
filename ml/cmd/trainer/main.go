// Comando trainer.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"ml-paralelo/internal/loader"
	"ml-paralelo/internal/ml"
	"ml-paralelo/internal/models"
)

func main() {
	dataPath := flag.String("data", "output/clean_records_sample.json", "ruta al CSV crudo o JSON de CleanRecords")
	numTrees := flag.Int("trees", 100, "número de árboles del bosque")
	numWorkers := flag.Int("workers", runtime.NumCPU(), "goroutines de entrenamiento")
	maxDepth := flag.Int("depth", 10, "profundidad máxima de cada árbol")
	outPath := flag.String("out", "output/train_results.json", "archivo de resultados")
	flag.Parse()

	fmt.Println("════════════════════════════════════════════════")
	fmt.Println("  Entrenamiento paralelo — Random Forest en Go")
	fmt.Println("════════════════════════════════════════════════")

	// ── 1. Cargar los datos ─────────────────────────────────────────────
	records, err := loadRecords(*dataPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error cargando datos: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Registros cargados: %d\n", len(records))

	// ── 2. Codificar features (label encoding de primary_type) ─────────
	ds := ml.BuildDataset(records)
	fmt.Printf("Features: %d | Tipos de crimen codificados: %d\n",
		ml.NumFeatures, len(ds.TypeEncoder))

	// ── 3. División estratificada 80/20 ─────────────────────────────────
	rng := rand.New(rand.NewSource(42))
	train, test := ds.StratifiedSplit(0.8, rng)
	fmt.Printf("Entrenamiento: %d muestras | Validación: %d muestras\n",
		len(train.Y), len(test.Y))

	// ── 4. Entrenamiento paralelo ───────────────────────────────────────
	cfg := ml.DefaultForestConfig()
	cfg.NumTrees = *numTrees
	cfg.NumWorkers = *numWorkers
	cfg.MaxDepth = *maxDepth

	fmt.Printf("\nEntrenando %d árboles con %d workers (goroutines)...\n",
		cfg.NumTrees, cfg.NumWorkers)
	forest := ml.Train(train, cfg)

	st := forest.TrainStats
	fmt.Printf("\n── Estadísticas del entrenamiento ──\n")
	fmt.Printf("Árboles entrenados : %d\n", st.NumTrees)
	fmt.Printf("Workers            : %d\n", st.NumWorkers)
	fmt.Printf("Tiempo             : %d ms\n", st.ElapsedMs)
	fmt.Printf("Árboles/segundo    : %.1f\n", st.TreesPerSec)
	fmt.Printf("Peso clase 0/1     : %.3f / %.3f\n", st.ClassWeight0, st.ClassWeight1)

	// ── 5. Evaluación sobre validación ──────────────────────────────────
	preds := forest.PredictBatch(test, cfg.NumWorkers)
	metrics := ml.Evaluate(test.Y, preds)

	fmt.Printf("\n── Métricas sobre validación (clase positiva: arrest) ──\n")
	fmt.Printf("Accuracy  : %.4f\n", metrics.Accuracy)
	fmt.Printf("Precision : %.4f\n", metrics.Precision)
	fmt.Printf("Recall    : %.4f\n", metrics.Recall)
	fmt.Printf("F1-score  : %.4f\n", metrics.F1)
	fmt.Printf("Confusión : TP=%d TN=%d FP=%d FN=%d\n",
		metrics.TP, metrics.TN, metrics.FP, metrics.FN)

	// ── 6. Persistir resultados ─────────────────────────────────────────
	results := map[string]interface{}{
		"train_stats": st,
		"metrics":     metrics,
		"config": map[string]interface{}{
			"num_trees":         cfg.NumTrees,
			"max_depth":         cfg.MaxDepth,
			"min_samples_split": cfg.MinSamplesSplit,
			"num_workers":       cfg.NumWorkers,
		},
	}
	if err := writeJSON(*outPath, results); err != nil {
		fmt.Fprintf(os.Stderr, "Error escribiendo resultados: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("\nResultados guardados en %s\n", *outPath)
}

// loadRecords carga CleanRecords desde un CSV crudo (vía ConcurrentLoader)
// o desde un JSON con el arreglo ya limpio.
func loadRecords(path string) ([]models.CleanRecord, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var records []models.CleanRecord
		if err := json.Unmarshal(data, &records); err != nil {
			return nil, err
		}
		return records, nil
	case ".csv":
		cfg := loader.DefaultConfig()
		ld := loader.New(cfg)
		ptrs, _, err := ld.LoadFile(path)
		if err != nil {
			return nil, err
		}
		records := make([]models.CleanRecord, len(ptrs))
		for i, p := range ptrs {
			records[i] = *p
		}
		return records, nil
	default:
		return nil, fmt.Errorf("formato no soportado: %s (usar .csv o .json)", path)
	}
}

func writeJSON(path string, v interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
