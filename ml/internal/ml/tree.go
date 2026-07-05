package ml

import (
	"math"
	"math/rand"
)

// TreeConfig contiene los hiperparámetros de un árbol individual.
type TreeConfig struct {
	MaxDepth        int     // profundidad máxima del árbol
	MinSamplesSplit int     // mínimo de muestras para dividir un nodo
	MaxFeatures     int     // features evaluadas por split (típicamente sqrt(NumFeatures))
	W0, W1          float64 // pesos de clase (balanceados)
}

// Node es un nodo del árbol de decisión. Si Left y Right son nil,
// el nodo es una hoja y Prediction contiene la clase mayoritaria ponderada.
type Node struct {
	FeatureIdx int     // índice de la feature usada en el split
	Threshold  float64 // umbral del split: izquierda si x < threshold
	Left       *Node
	Right      *Node
	Prediction int // clase predicha (solo en hojas)
}

// DecisionTree es un árbol entrenado.
type DecisionTree struct {
	Root *Node
}

// Predict recorre el árbol y devuelve la clase predicha para x.
func (t *DecisionTree) Predict(x [NumFeatures]float64) int {
	n := t.Root
	for n.Left != nil {
		if x[n.FeatureIdx] < n.Threshold {
			n = n.Left
		} else {
			n = n.Right
		}
	}
	return n.Prediction
}

func buildTree(ds *Dataset, idx []int, cfg TreeConfig, rng *rand.Rand) *DecisionTree {
	return &DecisionTree{Root: buildNode(ds, idx, cfg, 0, rng)}
}

func buildNode(ds *Dataset, idx []int, cfg TreeConfig, depth int, rng *rand.Rand) *Node {
	// criterios de parada: profundidad máxima, pocas muestras o nodo puro
	if depth >= cfg.MaxDepth || len(idx) < cfg.MinSamplesSplit || isPure(ds, idx) {
		return &Node{Prediction: weightedMajority(ds, idx, cfg)}
	}

	bestFeat, bestThr, bestGini := -1, 0.0, math.Inf(1)

	// selección aleatoria de MaxFeatures features candidatas (sin repetición)
	feats := rng.Perm(NumFeatures)[:cfg.MaxFeatures]

	for _, f := range feats {
		// candidatos de umbral: cuantiles aleatorios de la feature
		thresholds := sampleThresholds(ds, idx, f, rng)
		for _, thr := range thresholds {
			g := weightedGiniSplit(ds, idx, f, thr, cfg)
			if g < bestGini {
				bestGini, bestFeat, bestThr = g, f, thr
			}
		}
	}

	if bestFeat < 0 {
		return &Node{Prediction: weightedMajority(ds, idx, cfg)}
	}

	// partición de los índices según el mejor split
	var left, right []int
	for _, i := range idx {
		if ds.X[i][bestFeat] < bestThr {
			left = append(left, i)
		} else {
			right = append(right, i)
		}
	}
	if len(left) == 0 || len(right) == 0 {
		return &Node{Prediction: weightedMajority(ds, idx, cfg)}
	}

	return &Node{
		FeatureIdx: bestFeat,
		Threshold:  bestThr,
		Left:       buildNode(ds, left, cfg, depth+1, rng),
		Right:      buildNode(ds, right, cfg, depth+1, rng),
	}
}

// isPure indica si todos los índices pertenecen a la misma clase.
func isPure(ds *Dataset, idx []int) bool {
	if len(idx) == 0 {
		return true
	}
	first := ds.Y[idx[0]]
	for _, i := range idx[1:] {
		if ds.Y[i] != first {
			return false
		}
	}
	return true
}

// weightedMajority devuelve la clase con mayor masa ponderada.
func weightedMajority(ds *Dataset, idx []int, cfg TreeConfig) int {
	var m0, m1 float64
	for _, i := range idx {
		if ds.Y[i] == 1 {
			m1 += cfg.W1
		} else {
			m0 += cfg.W0
		}
	}
	if m1 > m0 {
		return 1
	}
	return 0
}

// sampleThresholds devuelve hasta 8 valores candidatos de umbral para la
// feature f, muestreados de los valores reales presentes en idx.
func sampleThresholds(ds *Dataset, idx []int, f int, rng *rand.Rand) []float64 {
	const maxCandidates = 8
	n := len(idx)
	if n == 0 {
		return nil
	}
	out := make([]float64, 0, maxCandidates)
	for k := 0; k < maxCandidates; k++ {
		i := idx[rng.Intn(n)]
		out = append(out, ds.X[i][f])
	}
	return out
}

func weightedGiniSplit(ds *Dataset, idx []int, f int, thr float64, cfg TreeConfig) float64 {
	var l0, l1, r0, r1 float64 // masas ponderadas por partición y clase
	for _, i := range idx {
		w := cfg.W0
		if ds.Y[i] == 1 {
			w = cfg.W1
		}
		if ds.X[i][f] < thr {
			if ds.Y[i] == 1 {
				l1 += w
			} else {
				l0 += w
			}
		} else {
			if ds.Y[i] == 1 {
				r1 += w
			} else {
				r0 += w
			}
		}
	}
	gini := func(a, b float64) float64 {
		tot := a + b
		if tot == 0 {
			return 0
		}
		pa, pb := a/tot, b/tot
		return 1 - pa*pa - pb*pb
	}
	lTot, rTot := l0+l1, r0+r1
	total := lTot + rTot
	if total == 0 {
		return math.Inf(1)
	}
	return (lTot/total)*gini(l0, l1) + (rTot/total)*gini(r0, r1)
}
