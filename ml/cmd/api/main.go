// Comando api.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"ml-paralelo/internal/api"
	"ml-paralelo/internal/models"
	"ml-paralelo/internal/storage"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	port := flag.String("port", envOr("PORT", "8080"), "puerto HTTP")
	nodesFlag := flag.String("nodes", envOr("NODES", "localhost:9001,localhost:9002,localhost:9003"), "nodos del clúster (host:puerto, separados por coma)")
	dataFile := flag.String("data", envOr("DATA_FILE", "output/clean_records_sample.json"), "JSON de CleanRecords")
	mongoURI := flag.String("mongo", envOr("MONGO_URI", ""), "URI de MongoDB")
	redisAddr := flag.String("redis", envOr("REDIS_ADDR", ""), "dirección de Redis")
	flag.Parse()

	nodes := strings.Split(*nodesFlag, ",")
	for i := range nodes {
		nodes[i] = strings.TrimSpace(nodes[i])
	}

	// ── Cargar los datos limpios producidos por el loader ───────────
	records, err := loadRecords(*dataFile)
	if err != nil {
		log.Printf("[api] advertencia: no se pudieron cargar datos de %s: %v", *dataFile, err)
	} else {
		log.Printf("[api] %d registros limpios cargados de %s", len(records), *dataFile)
	}

	srv := api.NewServer(api.Config{
		Nodes:     nodes,
		Mongo:     storage.NewMongo(*mongoURI, envOr("MONGO_DB", "chicago_crimes")),
		Cache:     storage.NewCache(*redisAddr, 10*time.Minute),
		JWTSecret: envOr("JWT_SECRET", "cc65-secret-dev"),
		AdminUser: envOr("ADMIN_USER", "admin"),
		AdminPass: envOr("ADMIN_PASS", "admin123"),
		UserUser:  envOr("USER_USER", "cliente"),
		UserPass:  envOr("USER_PASS", "cliente123"),
		Records:   records,
	})

	// ── Entrenamiento automático al arrancar (opcional) ─────────────
	if envOr("AUTO_TRAIN", "false") == "true" {
		go autoTrain(*port)
	}

	addr := ":" + *port
	log.Printf("[api] escuchando en %s | nodos del clúster: %v", addr, nodes)
	if err := http.ListenAndServe(addr, srv.Router()); err != nil {
		fmt.Fprintf(os.Stderr, "error del servidor: %v\n", err)
		os.Exit(1)
	}
}

func autoTrain(port string) {
	time.Sleep(3 * time.Second)
	base := "http://localhost:" + port

	body := strings.NewReader(fmt.Sprintf(`{"username":%q,"password":%q}`,
		envOr("ADMIN_USER", "admin"), envOr("ADMIN_PASS", "admin123")))
	resp, err := http.Post(base+"/api/login", "application/json", body)
	if err != nil {
		log.Printf("[api] auto-train: login falló: %v", err)
		return
	}
	defer resp.Body.Close()
	var lr struct {
		Token string `json:"token"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&lr)

	req, _ := http.NewRequest("POST", base+"/api/train", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+lr.Token)
	req.Header.Set("Content-Type", "application/json")
	r2, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[api] auto-train: error: %v", err)
		return
	}
	defer r2.Body.Close()
	log.Printf("[api] auto-train: status %s", r2.Status)
}

func loadRecords(path string) ([]models.CleanRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var records []models.CleanRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	return records, nil
}
