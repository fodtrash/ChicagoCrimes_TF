// Package cleaner implementa la limpieza y validación de registros CSV.
// Cada fila pasa por reglas de validación antes de ser aceptada.
package cleaner

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"crimes-loader/data/internal/models"
)

// Formato de fecha usado en el dataset de Chicago: "01/02/2006 03:04:05 PM"
const chicagoDateFormat = "01/02/2006 03:04:05 PM"

// Índices de columna del CSV (basados en la imagen del dataset)
const (
	colID                  = 0
	colCaseNumber          = 1
	colDate                = 2
	colBlock               = 3
	colIUCR                = 4
	colPrimaryType         = 5
	colDescription         = 6
	colLocationDescription = 7
	colArrest              = 8
	colDomestic            = 9
	colBeat                = 10
	colDistrict            = 11
	colWard                = 12
	colCommunityArea       = 13
	colFBICode             = 14
	colXCoordinate         = 15
	colYCoordinate         = 16
	colYear                = 17
	colUpdatedOn           = 18
	colLatitude            = 19
	colLongitude           = 20
	colLocation            = 21
	minColumns             = 20 // mínimo de columnas para considerar válida la fila
)

// ParseAndClean convierte una slice de campos CSV en un CleanRecord validado.
// Retorna nil y un error descriptivo si la fila no pasa validación.
func ParseAndClean(fields []string, lineNum int) (*models.CleanRecord, *models.ParseError) {
	if len(fields) < minColumns {
		return nil, &models.ParseError{
			Line:   lineNum,
			Field:  "row",
			Reason: fmt.Sprintf("columnas insuficientes: esperadas >=%d, obtenidas %d", minColumns, len(fields)),
		}
	}

	// --- ID ---
	id, err := strconv.ParseInt(strings.TrimSpace(fields[colID]), 10, 64)
	if err != nil || id <= 0 {
		return nil, &models.ParseError{Line: lineNum, Field: "id", Value: fields[colID], Reason: "id inválido"}
	}

	// --- Fecha ---
	rawDate := strings.TrimSpace(fields[colDate])
	date, err := time.Parse(chicagoDateFormat, rawDate)
	if err != nil {
		return nil, &models.ParseError{Line: lineNum, Field: "date", Value: rawDate, Reason: "fecha inválida: " + err.Error()}
	}

	// --- Coordenadas ---
	lat, lonErr1 := parseOptionalFloat(fields[colLatitude])
	lon, lonErr2 := parseOptionalFloat(fields[colLongitude])
	// Registros sin coordenadas son válidos (algunos crímenes no tienen ubicación)
	if lonErr1 != nil || lonErr2 != nil {
		lat, lon = 0, 0
	}
	// Validación geográfica: Chicago está entre lat 41.6-42.1, lon -87.9 a -87.5
	if lat != 0 && lon != 0 {
		if lat < 41.6 || lat > 42.1 || lon < -87.9 || lon > -87.5 {
			lat, lon = 0, 0 // descarta coordenada fuera de rango sin rechazar el registro
		}
	}

	// --- Año ---
	year, err := strconv.Atoi(strings.TrimSpace(fields[colYear]))
	if err != nil || year < 2001 || year > 2030 {
		return nil, &models.ParseError{Line: lineNum, Field: "year", Value: fields[colYear], Reason: "año fuera de rango"}
	}

	// --- District, Beat, Community Area ---
	district, _ := parseOptionalInt(fields[colDistrict])
	beat, _ := parseOptionalInt(fields[colBeat])
	communityArea, _ := parseOptionalInt(fields[colCommunityArea])

	// --- Tipo primario (campo clave para ML) ---
	primaryType := strings.TrimSpace(fields[colPrimaryType])
	if primaryType == "" {
		return nil, &models.ParseError{Line: lineNum, Field: "primary_type", Reason: "tipo de crimen vacío"}
	}
	primaryType = strings.ToUpper(primaryType)

	// --- Booleanos ---
	arrest := parseBool(fields[colArrest])
	domestic := parseBool(fields[colDomestic])

	// --- Location description ---
	locationDesc := strings.TrimSpace(fields[colLocationDescription])

	record := &models.CleanRecord{
		ID:                  id,
		Hour:                date.Hour(),
		DayOfWeek:           int(date.Weekday()),
		Month:               int(date.Month()),
		Year:                year,
		District:            district,
		CommunityArea:       communityArea,
		Beat:                beat,
		PrimaryType:         primaryType,
		Arrest:              arrest,
		Domestic:            domestic,
		Latitude:            lat,
		Longitude:           lon,
		LocationDescription: locationDesc,
	}

	return record, nil
}

// IsHeaderRow detecta si la fila es el encabezado CSV.
func IsHeaderRow(fields []string) bool {
	if len(fields) == 0 {
		return false
	}
	first := strings.ToLower(strings.TrimSpace(fields[0]))
	return first == "id" || first == "case_number" || strings.Contains(first, "case")
}

// --- helpers internos ---

func parseOptionalFloat(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	return strconv.ParseFloat(s, 64)
}

func parseOptionalInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	v, err := strconv.Atoi(s)
	return v, err
}

func parseBool(s string) bool {
	s = strings.ToUpper(strings.TrimSpace(s))
	return s == "TRUE" || s == "1" || s == "Y" || s == "YES"
}
