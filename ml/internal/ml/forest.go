package ml

import (
	"fmt"
	"math"
	"math/rand"
	"runtime"
	"sync"
	"time"
)

// ForestConfig contiene los hiperparámetros del Random Forest y la
// configuración de paralelización.
type ForestConfig struct {
	NumTrees        int // número total de árboles del bosque
	MaxDepth        int // profundidad máxima de cada árbol
	MinSamplesSplit int // mínimo de muestras para dividir un nodo
	NumWorkers      int // goroutines de entrenamiento (0 = NumCPU)
	Seed            int64
	Verbose         bool
}

func DefaultForestConfig() ForestConfig {
	return ForestConfig{
		NumTrees:        100,
		MaxDepth:        10,
		MinSamplesSplit: 20,
		NumWorkers:      runtime.NumCPU(),
		Seed:            42,
		Verbose:         true,
	}
}

// RandomForest es el modelo entrenado: una colección de árboles
// que predicen por votación mayoritaria.
type RandomForest struct {
	Trees  []*DecisionTree
	Config ForestConfig
	// TrainStats registra las métricas del entrenamiento paralelo.
	TrainStats TrainStats
}

// TrainStats contiene las métricas de la ejecución del entrenamiento.
type TrainStats struct {
	NumTrees      int           `json:"num_trees"`
	NumWorkers    int           `json:"num_workers"`
	TrainSamples  int           `json:"train_samples"`
	ElapsedMs     int64         `json:"elapsed_ms"`
	TreesPerSec   float64       `json:"trees_per_sec"`
	ClassWeight0  float64       `json:"class_weight_0"`
	ClassWeight1  float64       `json:"class_weight_1"`
	Elapsed       time.Duration `json:"-"`
}

func Train(ds *Dataset, cfg ForestConfig) *RandomForest {
	if cfg.NumWorkers <= 0 {
		cfg.NumWorkers = runtime.NumCPU()
	}
	w0, w1 := ds.ClassWeights()
	treeCfg := TreeConfig{
		MaxDepth:        cfg.MaxDepth,
		MinSamplesSplit: cfg.MinSamplesSplit,
		MaxFeatures:     int(math.Sqrt(float64(NumFeatures))) + 1, // ≈ 3
		W0:              w0,
		W1:              w1,
	}

	start := time.Now()

	// ── Canales del pipeline ────────────────────────────────────────────
	taskCh := make(chan int, cfg.NumTrees)        // tareas: índice del árbol
	treeCh := make(chan *DecisionTree, cfg.NumTrees) // resultados: árboles

	// ── Productor: carga todas las tareas y cierra el canal ────────────
	for i := 0; i < cfg.NumTrees; i++ {
		taskCh <- i
	}
	close(taskCh)

	// ── Worker pool: cada goroutine entrena árboles hasta agotar tareas ─
	var wg sync.WaitGroup
	for w := 0; w < cfg.NumWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(cfg.Seed + int64(workerID)))
			for treeIdx := range taskCh {
				sample := bootstrapSample(ds, rng)         // muestra con reemplazo
				tree := buildTree(ds, sample, treeCfg, rng) // construcción independiente
				treeCh <- tree
				if cfg.Verbose && (treeIdx+1)%20 == 0 {
					fmt.Printf("  [worker %d] árbol %d completado\n", workerID, treeIdx+1)
				}
			}
		}(w)
	}

	// ── Cierre del canal de resultados cuando todos los workers terminen ─
	go func() {
		wg.Wait()
		close(treeCh)
	}()

	// ── Agregador: recolecta los árboles del canal ──────────────────────
	forest := &RandomForest{Config: cfg}
	for tree := range treeCh {
		forest.Trees = append(forest.Trees, tree)
	}

	elapsed := time.Since(start)
	forest.TrainStats = TrainStats{
		NumTrees:     len(forest.Trees),
		NumWorkers:   cfg.NumWorkers,
		TrainSamples: len(ds.Y),
		ElapsedMs:    elapsed.Milliseconds(),
		TreesPerSec:  float64(len(forest.Trees)) / elapsed.Seconds(),
		ClassWeight0: w0,
		ClassWeight1: w1,
		Elapsed:      elapsed,
	}
	return forest
}

func bootstrapSample(ds *Dataset, rng *rand.Rand) []int {
	n := len(ds.Y)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = rng.Intn(n)
	}
	return idx
}

// Predict clasifica una muestra por votación mayoritaria entre
// todos los árboles del bosque.
func (f *RandomForest) Predict(x [NumFeatures]float64) int {
	votes := 0
	for _, t := range f.Trees {
		votes += t.Predict(x)
	}
	if votes*2 > len(f.Trees) {
		return 1
	}
	return 0
}

func (f *RandomForest) PredictBatch(ds *Dataset, numWorkers int) []int {
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
	preds := make([]int, len(ds.Y))
	var wg sync.WaitGroup
	chunk := (len(ds.Y) + numWorkers - 1) / numWorkers
	for w := 0; w < numWorkers; w++ {
		lo := w * chunk
		hi := lo + chunk
		if hi > len(ds.Y) {
			hi = len(ds.Y)
		}
		if lo >= hi {
			break
		}
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			for i := lo; i < hi; i++ {
				preds[i] = f.Predict(ds.X[i])
			}
		}(lo, hi)
	}
	wg.Wait()
	return preds
}
