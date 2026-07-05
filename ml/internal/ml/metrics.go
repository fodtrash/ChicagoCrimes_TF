package ml

// Metrics contiene las métricas de evaluación del clasificador
// sobre la clase positiva (arrest = true).
type Metrics struct {
	Accuracy  float64 `json:"accuracy"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1_score"`
	// Matriz de confusión
	TP int `json:"true_positives"`
	TN int `json:"true_negatives"`
	FP int `json:"false_positives"`
	FN int `json:"false_negatives"`
}

// Evaluate calcula accuracy, precision, recall y F1-score de la
// clase positiva comparando las predicciones con las etiquetas reales.
func Evaluate(yTrue, yPred []int) Metrics {
	var m Metrics
	for i := range yTrue {
		switch {
		case yTrue[i] == 1 && yPred[i] == 1:
			m.TP++
		case yTrue[i] == 0 && yPred[i] == 0:
			m.TN++
		case yTrue[i] == 0 && yPred[i] == 1:
			m.FP++
		case yTrue[i] == 1 && yPred[i] == 0:
			m.FN++
		}
	}
	total := float64(m.TP + m.TN + m.FP + m.FN)
	if total > 0 {
		m.Accuracy = float64(m.TP+m.TN) / total
	}
	if m.TP+m.FP > 0 {
		m.Precision = float64(m.TP) / float64(m.TP+m.FP)
	}
	if m.TP+m.FN > 0 {
		m.Recall = float64(m.TP) / float64(m.TP+m.FN)
	}
	if m.Precision+m.Recall > 0 {
		m.F1 = 2 * m.Precision * m.Recall / (m.Precision + m.Recall)
	}
	return m
}
