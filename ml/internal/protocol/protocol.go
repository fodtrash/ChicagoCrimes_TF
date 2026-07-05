// Package protocol define las estructuras serializables que viajan.
package protocol

import (
	"encoding/gob"
	"io"

	"ml-paralelo/internal/ml"
)

type TrainTask struct {
	NodeID          int
	NumTrees        int
	MaxDepth        int
	MinSamplesSplit int
	Seed            int64
	// Datos de entrenamiento codificados (features y etiquetas).
	X [][ml.NumFeatures]float64
	Y []int
}

type TrainResult struct {
	NodeID     int
	Trees      []*ml.DecisionTree
	ElapsedMs  int64
	NumWorkers int    // goroutines usadas dentro del nodo
	Error      string // no vacío si el entrenamiento local falló
}

// WriteTask serializa una TrainTask sobre el stream (conexión TCP).
func WriteTask(w io.Writer, t *TrainTask) error {
	return gob.NewEncoder(w).Encode(t)
}

// ReadTask deserializa una TrainTask desde el stream.
func ReadTask(r io.Reader) (*TrainTask, error) {
	var t TrainTask
	if err := gob.NewDecoder(r).Decode(&t); err != nil {
		return nil, err
	}
	return &t, nil
}

// WriteResult serializa un TrainResult sobre el stream.
func WriteResult(w io.Writer, res *TrainResult) error {
	return gob.NewEncoder(w).Encode(res)
}

// ReadResult deserializa un TrainResult desde el stream.
func ReadResult(r io.Reader) (*TrainResult, error) {
	var res TrainResult
	if err := gob.NewDecoder(r).Decode(&res); err != nil {
		return nil, err
	}
	return &res, nil
}
