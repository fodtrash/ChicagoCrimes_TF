// Package models define las estructuras de datos del dataset
// Chicago Crimes - 2001 to Present (Chicago Data Portal)
package models

import "time"

// CrimeRecord representa un registro de crimen del dataset de Chicago.
// Los campos coinciden exactamente con las columnas del Chicago Data Portal.
type CrimeRecord struct {
	ID                  int64     `json:"id"`
	CaseNumber          string    `json:"case_number"`
	Date                time.Time `json:"date"`
	Block               string    `json:"block"`
	IUCR                string    `json:"iucr"`
	PrimaryType         string    `json:"primary_type"`
	Description         string    `json:"description"`
	LocationDescription string    `json:"location_description"`
	Arrest              bool      `json:"arrest"`
	Domestic            bool      `json:"domestic"`
	Beat                int       `json:"beat"`
	District            int       `json:"district"`
	Ward                int       `json:"ward"`
	CommunityArea       int       `json:"community_area"`
	FBICode             string    `json:"fbi_code"`
	XCoordinate         float64   `json:"x_coordinate"`
	YCoordinate         float64   `json:"y_coordinate"`
	Year                int       `json:"year"`
	UpdatedOn           time.Time `json:"updated_on"`
	Latitude            float64   `json:"latitude"`
	Longitude           float64   `json:"longitude"`
	// Campo derivado para análisis de franja horaria (0-23)
	Hour int `json:"hour"`
}

// CleanRecord es el registro validado y listo para entrenamiento ML.
// Contiene solo los campos relevantes para el modelo predictivo.
type CleanRecord struct {
	ID                  int64   `json:"id"`
	Hour                int     `json:"hour"`       // Franja horaria (0-23)
	DayOfWeek           int     `json:"day_of_week"` // 0=Domingo ... 6=Sábado
	Month               int     `json:"month"`
	Year                int     `json:"year"`
	District            int     `json:"district"`
	CommunityArea       int     `json:"community_area"`
	Beat                int     `json:"beat"`
	PrimaryType         string  `json:"primary_type"`
	Arrest              bool    `json:"arrest"`
	Domestic            bool    `json:"domestic"`
	Latitude            float64 `json:"latitude"`
	Longitude           float64 `json:"longitude"`
	LocationDescription string  `json:"location_description"`
}

// LoadStats recopila métricas del proceso de carga concurrente.
type LoadStats struct {
	TotalLines     int64         `json:"total_lines"`
	ValidRecords   int64         `json:"valid_records"`
	InvalidRecords int64         `json:"invalid_records"`
	WorkersUsed    int           `json:"workers_used"`
	ChunkSize      int           `json:"chunk_size"`
	ElapsedMs      int64         `json:"elapsed_ms"`
	RecordsPerSec  float64       `json:"records_per_sec"`
	ErrorSummary   map[string]int `json:"error_summary"`
}

// ParseError describe un error de parseo con contexto.
type ParseError struct {
	Line    int
	Field   string
	Value   string
	Reason  string
}

func (e *ParseError) Error() string {
	return e.Reason
}
