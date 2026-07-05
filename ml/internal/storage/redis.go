package storage

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

type Cache struct {
	rdb *redis.Client
	ttl time.Duration
}

// NewCache conecta a Redis. Devuelve nil si no está disponible: la API
// opera sin caché (todas las respuestas serán MISS).
func NewCache(addr string, ttl time.Duration) *Cache {
	if addr == "" {
		return nil
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("[redis] no disponible (%v): la API opera sin caché", err)
		return nil
	}
	log.Printf("[redis] conectado a %s (TTL=%s)", addr, ttl)
	return &Cache{rdb: rdb, ttl: ttl}
}

// Get devuelve el valor cacheado y true (HIT), o "" y false (MISS).
func (c *Cache) Get(key string) (string, bool) {
	if c == nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		return "", false
	}
	return val, true
}

// Set guarda el valor con el TTL configurado.
func (c *Cache) Set(key, value string) {
	if c == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := c.rdb.Set(ctx, key, value, c.ttl).Err(); err != nil {
		log.Printf("[redis] error en SET %s: %v", key, err)
	}
}

// Healthy indica si la conexión sigue viva.
func (c *Cache) Healthy() bool {
	if c == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return c.rdb.Ping(ctx).Err() == nil
}
