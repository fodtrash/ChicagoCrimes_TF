// Package api implementa el servicio REST del sistema. La API cumple.
package api

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"ml-paralelo/internal/cluster"
	"ml-paralelo/internal/ml"
	"ml-paralelo/internal/models"
	"ml-paralelo/internal/storage"
)

type Server struct {
	mu      sync.RWMutex
	forest  *ml.RandomForest
	encoder map[string]int // label encoding de primary_type
	metrics *ml.Metrics    // métricas del último entrenamiento

	records []models.CleanRecord // datos limpios cargados al inicio
	distDefaults map[int]districtDefault
	coord        *cluster.Coordinator
	mongo   *storage.Mongo
	cache   *storage.Cache

	jwtSecret string
	adminUser string
	adminPass string
	userUser  string
	userPass  string

	// Contadores de operación (atomics: sin locks en la ruta caliente).
	predCount   atomic.Int64
	cacheHits   atomic.Int64
	cacheMisses atomic.Int64
	latencySum  atomic.Int64 // microsegundos acumulados
	startedAt   time.Time

	lastNodeStats []cluster.NodeStat
	lastTrainMs   int64

	sys *sysMetrics // muestreador de CPU/memoria (uso CPU del panel admin)
}

// districtDefault guarda la combinación beat/área comunitaria más
// frecuente observada en un distrito.
type districtDefault struct {
	Beat          int
	CommunityArea int
}

// computeDistrictDefaults calcula, para cada distrito, el beat y el
// área comunitaria con mayor frecuencia en los registros limpios.
func computeDistrictDefaults(records []models.CleanRecord) map[int]districtDefault {
	beatCount := map[int]map[int]int{}
	caCount := map[int]map[int]int{}
	for _, r := range records {
		if beatCount[r.District] == nil {
			beatCount[r.District] = map[int]int{}
			caCount[r.District] = map[int]int{}
		}
		beatCount[r.District][r.Beat]++
		caCount[r.District][r.CommunityArea]++
	}
	mode := func(m map[int]int) int {
		best, bestN := 0, -1
		for v, n := range m {
			if n > bestN {
				best, bestN = v, n
			}
		}
		return best
	}
	out := make(map[int]districtDefault, len(beatCount))
	for d := range beatCount {
		out[d] = districtDefault{Beat: mode(beatCount[d]), CommunityArea: mode(caCount[d])}
	}
	return out
}

// Config agrupa los parámetros de arranque de la API.
type Config struct {
	Nodes     []string
	Mongo     *storage.Mongo
	Cache     *storage.Cache
	JWTSecret string
	AdminUser string
	AdminPass string
	UserUser  string // usuario con rol cliente (solo predicción e impacto)
	UserPass  string
	Records   []models.CleanRecord
}

// NewServer construye el servidor y registra las rutas.
func NewServer(cfg Config) *Server {
	s := &Server{
		coord:     cluster.NewCoordinator(cfg.Nodes),
		mongo:     cfg.Mongo,
		cache:     cfg.Cache,
		jwtSecret: cfg.JWTSecret,
		adminUser: cfg.AdminUser,
		adminPass: cfg.AdminPass,
		userUser:  cfg.UserUser,
		userPass:  cfg.UserPass,
		records:   cfg.Records,
		startedAt: time.Now(),
		sys:       newSysMetrics(),
	}
	s.distDefaults = computeDistrictDefaults(cfg.Records)
	return s
}

// Router devuelve el mux con todos los endpoints.
func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/train", s.authMiddleware("admin", s.handleTrain))
	mux.HandleFunc("POST /api/predict", s.handlePredict)
	mux.HandleFunc("GET /api/metrics", s.handleMetrics)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/predictions/recent", s.handleRecentPredictions)
	mux.HandleFunc("GET /ws/metrics", s.handleWSMetrics)
	return corsMiddleware(mux)
}

// ── Handlers ────────────────────────────────────────────────────────

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido"})
		return
	}
	// Dos cuentas: admin (panel de métricas + reentrenamiento) y
	// cliente (solo predicción y visualización de impacto).
	var role string
	switch {
	case req.Username == s.adminUser && req.Password == s.adminPass:
		role = "admin"
	case req.Username == s.userUser && req.Password == s.userPass:
		role = "cliente"
	default:
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "credenciales inválidas"})
		return
	}
	token := signJWT(s.jwtSecret, req.Username, role, 8*time.Hour)
	writeJSON(w, http.StatusOK, map[string]string{"token": token, "user": req.Username, "role": role})
}

type trainRequest struct {
	NumTrees int `json:"num_trees"`
	MaxDepth int `json:"max_depth"`
}

func (s *Server) handleTrain(w http.ResponseWriter, r *http.Request) {
	var req trainRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // cuerpo opcional

	cfg := ml.DefaultForestConfig()
	cfg.Verbose = false
	if req.NumTrees > 0 {
		cfg.NumTrees = req.NumTrees
	}
	if req.MaxDepth > 0 {
		cfg.MaxDepth = req.MaxDepth
	}

	if len(s.records) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no hay datos cargados"})
		return
	}

	ds := ml.BuildDataset(s.records)
	rng := rand.New(rand.NewSource(42))
	train, test := ds.StratifiedSplit(0.8, rng)

	log.Printf("[api] entrenamiento distribuido: %d árboles entre %d nodos",
		cfg.NumTrees, len(s.coord.Nodes))
	forest, nodeStats, err := s.coord.TrainDistributed(train, cfg)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	// Evaluación sobre validación con inferencia paralela por lotes.
	preds := forest.PredictBatch(test, 0)
	m := ml.Evaluate(test.Y, preds)

	// Intercambio atómico del modelo activo.
	s.mu.Lock()
	s.forest = forest
	s.encoder = ds.TypeEncoder
	s.metrics = &m
	s.lastNodeStats = nodeStats
	s.lastTrainMs = forest.TrainStats.ElapsedMs
	s.mu.Unlock()

	// Persistencia de metadatos del entrenamiento en MongoDB.
	s.mongo.SaveTraining(map[string]interface{}{
		"num_trees":  forest.TrainStats.NumTrees,
		"elapsed_ms": forest.TrainStats.ElapsedMs,
		"nodes":      nodeStats,
		"metrics":    m,
		"samples":    forest.TrainStats.TrainSamples,
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "modelo entrenado",
		"train_stats": forest.TrainStats,
		"node_stats":  nodeStats,
		"metrics":     m,
	})
}

// PredictRequest es la consulta de predicción que envía el cliente.
type PredictRequest struct {
	Hour          int    `json:"hour"`
	District      int    `json:"district"`
	PrimaryType   string `json:"primary_type"`
	Domestic      bool   `json:"domestic"`
	DayOfWeek     int    `json:"day_of_week"`
	Month         int    `json:"month"`
	CommunityArea int    `json:"community_area"`
	Beat          int    `json:"beat"`
}

func (s *Server) handlePredict(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	var req PredictRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "JSON inválido"})
		return
	}

	s.mu.RLock()
	forest, encoder := s.forest, s.encoder
	s.mu.RUnlock()
	if forest == nil {
		writeJSON(w, http.StatusServiceUnavailable,
			map[string]string{"error": "el modelo aún no está entrenado: use POST /api/train"})
		return
	}

	if def, ok := s.distDefaults[req.District]; ok {
		if req.Beat == 0 {
			req.Beat = def.Beat
		}
		if req.CommunityArea == 0 {
			req.CommunityArea = def.CommunityArea
		}
	}

	key := fmt.Sprintf("pred:%d:%d:%s:%t:%d:%d:%d:%d",
		req.Hour, req.District, req.PrimaryType, req.Domestic,
		req.DayOfWeek, req.Month, req.CommunityArea, req.Beat)

	if cached, ok := s.cache.Get(key); ok {
		s.cacheHits.Add(1)
		s.trackLatency(start)
		w.Header().Set("X-Cache", "HIT")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(cached))
		return
	}
	s.cacheMisses.Add(1)

	typeCode, known := encoder[req.PrimaryType]
	x := [ml.NumFeatures]float64{
		float64(req.Hour), float64(req.District), float64(typeCode),
		boolToF(req.Domestic), float64(req.DayOfWeek), float64(req.Month),
		float64(req.CommunityArea), float64(req.Beat),
	}

	// Probabilidad = fracción de árboles que votan por la clase 1.
	votes := 0
	for _, t := range forest.Trees {
		votes += t.Predict(x)
	}
	prob := float64(votes) / float64(len(forest.Trees))
	risk := "bajo"
	switch {
	case prob >= 0.66:
		risk = "alto"
	case prob >= 0.33:
		risk = "medio"
	}

	resp := map[string]interface{}{
		"arrest_probability":  prob,
		"risk_level":          risk,
		"known_crime_type":    known,
		"model_trees":         len(forest.Trees),
		"district":            req.District,
		"used_beat":           req.Beat,
		"used_community_area": req.CommunityArea,
		"latency_ms":          float64(time.Since(start).Microseconds()) / 1000.0,
	}
	body, _ := json.Marshal(resp)
	s.cache.Set(key, string(body))

	s.mongo.SavePrediction(map[string]interface{}{
		"query": req, "probability": prob, "risk": risk,
	})

	s.predCount.Add(1)
	s.trackLatency(start)
	w.Header().Set("X-Cache", "MISS")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// clusterMetrics arma la vista de métricas que consumen /api/metrics y
// el canal WebSocket.
func (s *Server) clusterMetrics() map[string]interface{} {
	s.mu.RLock()
	nodeStats := s.lastNodeStats
	trainMs := s.lastTrainMs
	m := s.metrics
	trained := s.forest != nil
	trees := 0
	if trained {
		trees = len(s.forest.Trees)
	}
	s.mu.RUnlock()

	alive := 0
	nodes := make([]map[string]interface{}, 0, len(s.coord.Nodes))
	for _, addr := range s.coord.Nodes {
		up := s.coord.Ping(addr)
		if up {
			alive++
		}
		nodes = append(nodes, map[string]interface{}{"addr": addr, "alive": up})
	}

	preds := s.predCount.Load()
	hits := s.cacheHits.Load()
	misses := s.cacheMisses.Load()
	var avgLatMs float64
	if total := preds + hits; total > 0 {
		avgLatMs = float64(s.latencySum.Load()) / float64(total) / 1000.0
	}

	out := map[string]interface{}{
		"nodes_total":      len(s.coord.Nodes),
		"nodes_alive":      alive,
		"nodes":            nodes,
		"model_trained":    trained,
		"model_trees":      trees,
		"last_train_ms":    trainMs,
		"last_node_stats":  nodeStats,
		"model_metrics":    m,
		"predictions":      preds,
		"cache_hits":       hits,
		"cache_misses":     misses,
		"avg_latency_ms":   avgLatMs,
		"mongo_healthy":    s.mongo.Healthy(),
		"redis_healthy":    s.cache.Healthy(),
		"uptime_seconds":   int(time.Since(s.startedAt).Seconds()),
		"timestamp":        time.Now().Format(time.RFC3339),
	}
	// uso de CPU (sistema y proceso), memoria y goroutines
	for k, v := range s.sys.snapshot() {
		out[k] = v
	}
	return out
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.clusterMetrics())
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRecentPredictions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.mongo.RecentPredictions(20))
}

// handleWSMetrics emite las métricas del clúster cada 2 segundos por
// WebSocket, en tiempo real, al panel de administración del frontend.
func (s *Server) handleWSMetrics(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrade(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer conn.Close()
	log.Printf("[ws] cliente conectado: %s", conn.RemoteAddr())

	done := make(chan struct{})
	go wsDrain(conn, done)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// primer envío inmediato
	if msg, err := json.Marshal(s.clusterMetrics()); err == nil {
		if wsWriteText(conn, msg) != nil {
			return
		}
	}
	for {
		select {
		case <-done:
			log.Printf("[ws] cliente desconectado: %s", conn.RemoteAddr())
			return
		case <-ticker.C:
			msg, err := json.Marshal(s.clusterMetrics())
			if err != nil {
				continue
			}
			if wsWriteText(conn, msg) != nil {
				return
			}
		}
	}
}

// ── Helpers ─────────────────────────────────────────────────────────

func (s *Server) trackLatency(start time.Time) {
	s.latencySum.Add(time.Since(start).Microseconds())
}

func boolToF(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Expose-Headers", "X-Cache")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
