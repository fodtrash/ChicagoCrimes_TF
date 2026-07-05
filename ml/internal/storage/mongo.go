// Package storage implementa los dos niveles de almacenamiento del
// sistema: persistente (MongoDB) y caché (Redis).
package storage

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type Mongo struct {
	client *mongo.Client
	db     *mongo.Database
}

// NewMongo establece la conexión mediante el driver oficial de Go.
// Devuelve nil (sin error fatal) si Mongo no está disponible.
func NewMongo(uri, dbName string) *Mongo {
	if uri == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		log.Printf("[mongo] no disponible (%v): la API opera sin persistencia", err)
		return nil
	}
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		log.Printf("[mongo] ping falló (%v): la API opera sin persistencia", err)
		return nil
	}
	log.Printf("[mongo] conectado a %s (db=%s)", uri, dbName)
	return &Mongo{client: client, db: client.Database(dbName)}
}

func (m *Mongo) SaveTraining(doc map[string]interface{}) {
	if m == nil {
		return
	}
	doc["timestamp"] = time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := m.db.Collection("trainings").InsertOne(ctx, doc); err != nil {
		log.Printf("[mongo] error guardando entrenamiento: %v", err)
	}
}

// SavePrediction persiste cada predicción servida (historial).
func (m *Mongo) SavePrediction(doc map[string]interface{}) {
	if m == nil {
		return
	}
	doc["timestamp"] = time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := m.db.Collection("predictions").InsertOne(ctx, doc); err != nil {
		log.Printf("[mongo] error guardando predicción: %v", err)
	}
}

// RecentPredictions devuelve las últimas n predicciones registradas.
func (m *Mongo) RecentPredictions(n int64) []map[string]interface{} {
	if m == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	opts := options.Find().SetSort(map[string]interface{}{"timestamp": -1}).SetLimit(n)
	cur, err := m.db.Collection("predictions").Find(ctx, map[string]interface{}{}, opts)
	if err != nil {
		return nil
	}
	var out []map[string]interface{}
	_ = cur.All(ctx, &out)
	return out
}

// Healthy indica si la conexión sigue viva.
func (m *Mongo) Healthy() bool {
	if m == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return m.client.Ping(ctx, readpref.Primary()) == nil
}
