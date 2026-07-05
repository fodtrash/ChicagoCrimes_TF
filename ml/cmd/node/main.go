// Comando node.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"runtime"
	"time"

	"ml-paralelo/internal/ml"
	"ml-paralelo/internal/protocol"
)

func main() {
	port := flag.Int("port", 9000, "puerto TCP de escucha")
	workers := flag.Int("workers", runtime.NumCPU(), "goroutines de entrenamiento locales")
	flag.Parse()

	addr := fmt.Sprintf(":%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "No se pudo escuchar en %s: %v\n", addr, err)
		os.Exit(1)
	}
	log.Printf("[node] escuchando en %s (workers locales: %d)", addr, *workers)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("[node] error aceptando conexión: %v", err)
			continue
		}
		// Cada conexión se atiende en su propia goroutine: el nodo puede
		// servir múltiples tareas simultáneas sin bloquear el listener.
		go handleConn(conn, *workers)
	}
}

func handleConn(conn net.Conn, workers int) {
	defer conn.Close()
	remote := conn.RemoteAddr().String()

	task, err := protocol.ReadTask(conn)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return
		}
		log.Printf("[node] error leyendo tarea de %s: %v", remote, err)
		return
	}
	log.Printf("[node] tarea recibida de %s: %d árboles, %d muestras, seed=%d",
		remote, task.NumTrees, len(task.Y), task.Seed)

	res := &protocol.TrainResult{NodeID: task.NodeID, NumWorkers: workers}
	start := time.Now()

	ds := &ml.Dataset{X: task.X, Y: task.Y}

	cfg := ml.ForestConfig{
		NumTrees:        task.NumTrees,
		MaxDepth:        task.MaxDepth,
		MinSamplesSplit: task.MinSamplesSplit,
		NumWorkers:      workers,
		Seed:            task.Seed,
		Verbose:         false,
	}

	forest := ml.Train(ds, cfg)
	res.Trees = forest.Trees
	res.ElapsedMs = time.Since(start).Milliseconds()

	if err := protocol.WriteResult(conn, res); err != nil {
		log.Printf("[node] error enviando resultado a %s: %v", remote, err)
		return
	}
	log.Printf("[node] %d árboles devueltos a %s en %d ms",
		len(res.Trees), remote, res.ElapsedMs)
}
