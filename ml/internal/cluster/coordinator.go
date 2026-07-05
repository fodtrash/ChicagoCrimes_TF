// Package cluster implementa el coordinador del clúster de nodos ML.
package cluster

import (
	"fmt"
	"log"
	"net"
	"time"

	"ml-paralelo/internal/ml"
	"ml-paralelo/internal/protocol"
)

// Coordinator conoce las direcciones de los nodos y los parámetros de
// robustez (timeouts) del despacho distribuido.
type Coordinator struct {
	Nodes       []string      // direcciones host:puerto de los nodos
	DialTimeout time.Duration // límite para establecer la conexión
	IOTimeout   time.Duration // límite total de envío+entrenamiento+respuesta
}

// NewCoordinator crea un coordinador con timeouts por defecto.
func NewCoordinator(nodes []string) *Coordinator {
	return &Coordinator{
		Nodes:       nodes,
		DialTimeout: 5 * time.Second,
		IOTimeout:   10 * time.Minute,
	}
}

// NodeStat resume la participación de un nodo en un entrenamiento.
type NodeStat struct {
	Addr      string `json:"addr"`
	NodeID    int    `json:"node_id"`
	Trees     int    `json:"trees"`
	ElapsedMs int64  `json:"elapsed_ms"`
	Workers   int    `json:"workers"`
	Error     string `json:"error,omitempty"`
}

func (c *Coordinator) TrainDistributed(ds *ml.Dataset, cfg ml.ForestConfig) (*ml.RandomForest, []NodeStat, error) {
	n := len(c.Nodes)
	if n == 0 {
		return nil, nil, fmt.Errorf("no hay nodos configurados")
	}

	// Partición equitativa: el último nodo absorbe el residuo de la
	// división entera.
	perNode := cfg.NumTrees / n
	tasks := make([]*protocol.TrainTask, n)
	for i := 0; i < n; i++ {
		trees := perNode
		if i == n-1 {
			trees = cfg.NumTrees - perNode*(n-1)
		}
		tasks[i] = &protocol.TrainTask{
			NodeID:          i,
			NumTrees:        trees,
			MaxDepth:        cfg.MaxDepth,
			MinSamplesSplit: cfg.MinSamplesSplit,
			// Semilla base por nodo: garantiza muestras bootstrap
			// distintas entre nodos, evitando árboles duplicados.
			Seed: 1000 + int64(i)*100,
			X:    ds.X,
			Y:    ds.Y,
		}
	}

	start := time.Now()
	resultCh := make(chan *protocol.TrainResult, n)
	errCh := make(chan NodeStat, n)

	// ── Despacho concurrente: una goroutine por nodo ────────────────
	for i, addr := range c.Nodes {
		go func(nodeID int, addr string, task *protocol.TrainTask) {
			res, err := c.dispatch(addr, task)
			if err != nil {
				// Los errores se reportan por un canal separado en
				// lugar de provocar un pánico.
				errCh <- NodeStat{Addr: addr, NodeID: nodeID, Error: err.Error()}
				return
			}
			res.NodeID = nodeID
			resultCh <- res
		}(i, addr, tasks[i])
	}

	// ── Agregador: recolecta resultados y ensambla el bosque ────────
	forest := &ml.RandomForest{Config: cfg}
	stats := make([]NodeStat, 0, n)
	var failed []NodeStat

	for received := 0; received < n; received++ {
		select {
		case res := <-resultCh:
			forest.Trees = append(forest.Trees, res.Trees...)
			stats = append(stats, NodeStat{
				Addr:      c.Nodes[res.NodeID],
				NodeID:    res.NodeID,
				Trees:     len(res.Trees),
				ElapsedMs: res.ElapsedMs,
				Workers:   res.NumWorkers,
			})
		case st := <-errCh:
			log.Printf("[coordinator] nodo %s falló: %s", st.Addr, st.Error)
			failed = append(failed, st)
		}
	}

	if len(failed) > 0 && len(stats) > 0 {
		for _, st := range failed {
			retryAddr := stats[0].Addr
			log.Printf("[coordinator] reasignando lote del nodo %d a %s", st.NodeID, retryAddr)
			task := tasks[st.NodeID]
			res, err := c.dispatch(retryAddr, task)
			if err != nil {
				stats = append(stats, st) // se registra el fallo definitivo
				continue
			}
			forest.Trees = append(forest.Trees, res.Trees...)
			stats = append(stats, NodeStat{
				Addr: retryAddr, NodeID: st.NodeID,
				Trees: len(res.Trees), ElapsedMs: res.ElapsedMs, Workers: res.NumWorkers,
			})
		}
	}

	if len(forest.Trees) == 0 {
		return nil, stats, fmt.Errorf("ningún nodo completó el entrenamiento")
	}

	elapsed := time.Since(start)
	w0, w1 := ds.ClassWeights()
	forest.TrainStats = ml.TrainStats{
		NumTrees:     len(forest.Trees),
		NumWorkers:   len(c.Nodes),
		TrainSamples: len(ds.Y),
		ElapsedMs:    elapsed.Milliseconds(),
		TreesPerSec:  float64(len(forest.Trees)) / elapsed.Seconds(),
		ClassWeight0: w0,
		ClassWeight1: w1,
		Elapsed:      elapsed,
	}
	return forest, stats, nil
}

// dispatch envía una tarea a un nodo y espera su resultado, aplicando
// timeouts sobre la conexión TCP para evitar bloqueos indefinidos.
func (c *Coordinator) dispatch(addr string, task *protocol.TrainTask) (*protocol.TrainResult, error) {
	conn, err := net.DialTimeout("tcp", addr, c.DialTimeout)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(c.IOTimeout))

	if err := protocol.WriteTask(conn, task); err != nil {
		return nil, fmt.Errorf("enviando tarea a %s: %w", addr, err)
	}
	res, err := protocol.ReadResult(conn)
	if err != nil {
		return nil, fmt.Errorf("leyendo resultado de %s: %w", addr, err)
	}
	if res.Error != "" {
		return nil, fmt.Errorf("nodo %s reportó error: %s", addr, res.Error)
	}
	return res, nil
}

// Ping verifica si un nodo acepta conexiones TCP (sonda de salud).
func (c *Coordinator) Ping(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
