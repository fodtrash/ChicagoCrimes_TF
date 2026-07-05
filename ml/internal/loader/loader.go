// Package loader implementa la carga paralela de datos desde CSV.
package loader

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"ml-paralelo/internal/models"
)

// Config contiene la configuración del loader.
type Config struct {
	NumWorkers int
	BufferSize int
}

// DefaultConfig devuelve la configuración por defecto del loader.
func DefaultConfig() Config {
	return Config{
		NumWorkers: 4,
		BufferSize: 1000,
	}
}

// Loader carga registros desde un archivo CSV.
type Loader struct {
	cfg Config
}

// New crea un nuevo loader con la configuración especificada.
func New(cfg Config) *Loader {
	return &Loader{cfg: cfg}
}

func (l *Loader) LoadFile(path string) ([]*models.CleanRecord, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("no se puede abrir archivo %s: %w", path, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)

	// Leer header
	header, err := reader.Read()
	if err != nil {
		return nil, 0, fmt.Errorf("error leyendo header: %w", err)
	}

	// Mapear nombres de columnas a índices
	colIndex := make(map[string]int)
	for i, name := range header {
		colIndex[strings.ToLower(name)] = i
	}

	var records []*models.CleanRecord
	errorCount := 0

	// Leer todos los registros
	for {
		row, err := reader.Read()
		if err != nil {
			break // EOF
		}

		record, err := parseRecord(row, colIndex)
		if err != nil {
			errorCount++
			continue
		}

		records = append(records, record)
	}

	return records, errorCount, nil
}

// parseRecord convierte una fila CSV en un CleanRecord.
// Extrae fields como Hour, DayOfWeek y Month de la columna Date.
func parseRecord(row []string, colIndex map[string]int) (*models.CleanRecord, error) {
	rec := &models.CleanRecord{}

	// Funciones auxiliares para obtener valores de columnas
	getInt := func(name string) (int, error) {
		idx, ok := colIndex[strings.ToLower(name)]
		if !ok || idx >= len(row) {
			return 0, fmt.Errorf("columna %s no encontrada", name)
		}
		val := strings.TrimSpace(row[idx])
		if val == "" {
			return 0, nil
		}
		return strconv.Atoi(val)
	}

	getString := func(name string) (string, error) {
		idx, ok := colIndex[strings.ToLower(name)]
		if !ok || idx >= len(row) {
			return "", fmt.Errorf("columna %s no encontrada", name)
		}
		return strings.TrimSpace(row[idx]), nil
	}

	getBool := func(name string) (bool, error) {
		idx, ok := colIndex[strings.ToLower(name)]
		if !ok || idx >= len(row) {
			return false, fmt.Errorf("columna %s no encontrada", name)
		}
		val := strings.ToLower(strings.TrimSpace(row[idx]))
		return val == "true" || val == "yes" || val == "1" || val == "t", nil
	}

	getDate := func(name string) (time.Time, error) {
		idx, ok := colIndex[strings.ToLower(name)]
		if !ok || idx >= len(row) {
			return time.Time{}, fmt.Errorf("columna %s no encontrada", name)
		}
		val := strings.TrimSpace(row[idx])
		// Formato esperado: "05/23/2026 12:00:00 AM"
		return time.Parse("01/02/2006 03:04:05 PM", val)
	}

	// Parsear campos del CSV de Chicago Data Portal
	var err error

	// Extraer fecha y derivar hour, day_of_week, month
	dateVal, err := getDate("date")
	if err != nil {
		return nil, err
	}
	rec.Hour = dateVal.Hour()
	rec.DayOfWeek = int(dateVal.Weekday()) // 0 = Sunday, ..., 6 = Saturday
	rec.Month = int(dateVal.Month())

	rec.District, err = getInt("district")
	if err != nil {
		return nil, err
	}
	rec.PrimaryType, err = getString("primary type")
	if err != nil {
		return nil, err
	}
	rec.Domestic, err = getBool("domestic")
	if err != nil {
		return nil, err
	}
	rec.CommunityArea, err = getInt("community area")
	if err != nil {
		return nil, err
	}
	rec.Beat, err = getInt("beat")
	if err != nil {
		return nil, err
	}
	rec.Arrest, err = getBool("arrest")
	if err != nil {
		return nil, err
	}

	return rec, nil
}
