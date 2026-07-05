// Package models define las estructuras de datos para el proyecto.
package models

// CleanRecord representa un registro limpio de crimen de Chicago,
// con features codificadas listo para ML.
type CleanRecord struct {
	Hour          int    `json:"hour"`
	District      int    `json:"district"`
	PrimaryType   string `json:"primary_type"`
	Domestic      bool   `json:"domestic"`
	DayOfWeek     int    `json:"day_of_week"`
	Month         int    `json:"month"`
	CommunityArea int    `json:"community_area"`
	Beat          int    `json:"beat"`
	Arrest        bool   `json:"arrest"`
}
