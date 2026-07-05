package loader_test

import (
	"encoding/csv"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"crimes-loader/data/internal/loader"
	"crimes-loader/data/internal/models"
)

// generateTestCSV crea un archivo CSV temporal con n registros válidos.
func generateTestCSV(t *testing.T, n int) string {
	t.Helper()
	f, err := os.CreateTemp("", "crimes_test_*.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	rng := rand.New(rand.NewSource(42))
	header := "ID,Case Number,Date,Block,IUCR,Primary Type,Description,Location Description," +
		"Arrest,Domestic,Beat,District,Ward,Community Area,FBI Code,X Coordinate," +
		"Y Coordinate,Year,Updated On,Latitude,Longitude,Location\n"
	f.WriteString(header)

	types := []string{"THEFT", "BATTERY", "ASSAULT", "ROBBERY"}
	for i := 1; i <= n; i++ {
		hour := rng.Intn(24)
		ampm, h12 := "AM", hour
		if hour >= 12 {
			ampm = "PM"
			if hour > 12 {
				h12 = hour - 12
			}
		}
		if h12 == 0 {
			h12 = 12
		}
		date := fmt.Sprintf("01/15/2019 %02d:00:00 %s", h12, ampm)
		lat := 41.65 + rng.Float64()*0.37
		lon := -87.87 + rng.Float64()*0.35
		line := fmt.Sprintf("%d,JE%06d,%s,123 N MAIN ST,0110,%s,SIMPLE,STREET,false,false,%d,%d,%d,%d,06,1160000,1880000,2019,01/15/2020 01:00:00 AM,%.6f,%.6f,\"(%.6f %.6f)\"\n",
			i, i, date, types[rng.Intn(len(types))],
			100+rng.Intn(10), 1+rng.Intn(22), 1+rng.Intn(50), 1+rng.Intn(77),
			lat, lon, lat, lon)
		f.WriteString(line)
	}
	return f.Name()
}

// TestLoadFile_Basic verifica que el loader carga registros correctamente.
func TestLoadFile_Basic(t *testing.T) {
	path := generateTestCSV(t, 1000)
	defer os.Remove(path)

	cfg := loader.Config{NumWorkers: 4, ChunkSize: 200, BufferSize: 16, Verbose: false}
	l := loader.New(cfg)
	records, stats, err := l.LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile error: %v", err)
	}
	if stats.ValidRecords == 0 {
		t.Fatal("expected valid records, got 0")
	}
	if len(records) != int(stats.ValidRecords) {
		t.Errorf("records slice len %d != stats.ValidRecords %d", len(records), stats.ValidRecords)
	}
	t.Logf("Loaded %d valid records in %d ms", stats.ValidRecords, stats.ElapsedMs)
}

// TestLoadFile_WorkerScaling verifica que más workers produce el mismo resultado.
func TestLoadFile_WorkerScaling(t *testing.T) {
	path := generateTestCSV(t, 5000)
	defer os.Remove(path)

	workerCounts := []int{1, 2, 4, 8}
	var baseCount int64

	for _, nw := range workerCounts {
		cfg := loader.Config{NumWorkers: nw, ChunkSize: 500, BufferSize: 16, Verbose: false}
		l := loader.New(cfg)
		_, stats, err := l.LoadFile(path)
		if err != nil {
			t.Fatalf("workers=%d: LoadFile error: %v", nw, err)
		}
		if baseCount == 0 {
			baseCount = stats.ValidRecords
		}
		if stats.ValidRecords != baseCount {
			t.Errorf("workers=%d: got %d records, expected %d", nw, stats.ValidRecords, baseCount)
		}
		t.Logf("workers=%d: %d records, %d ms", nw, stats.ValidRecords, stats.ElapsedMs)
	}
}

// TestLoadFile_InvalidRows verifica que las filas malformadas son rechazadas.
func TestLoadFile_InvalidRows(t *testing.T) {
	f, _ := os.CreateTemp("", "crimes_invalid_*.csv")
	defer os.Remove(f.Name())
	defer f.Close()

	f.WriteString("ID,Case Number,Date,Block,IUCR,Primary Type,Description,Location Description,Arrest,Domestic,Beat,District,Ward,Community Area,FBI Code,X Coordinate,Y Coordinate,Year,Updated On,Latitude,Longitude,Location\n")
	// Fila válida
	f.WriteString("1,JE000001,01/15/2019 02:00:00 AM,100 N MAIN ST,0110,THEFT,SIMPLE,STREET,false,false,123,8,42,23,06,1160000,1880000,2019,01/15/2020 01:00:00 AM,41.8000,-87.7000,\"(41.8 -87.7)\"\n")
	// Fila con ID inválido
	f.WriteString("abc,JE000002,01/15/2019 02:00:00 AM,100 N MAIN ST,0110,THEFT,SIMPLE,STREET,false,false,123,8,42,23,06,1160000,1880000,2019,01/15/2020 01:00:00 AM,41.8000,-87.7000,\"(41.8 -87.7)\"\n")
	// Fila con fecha inválida
	f.WriteString("3,JE000003,NOT_A_DATE,100 N MAIN ST,0110,THEFT,SIMPLE,STREET,false,false,123,8,42,23,06,1160000,1880000,2019,01/15/2020 01:00:00 AM,41.8000,-87.7000,\"(41.8 -87.7)\"\n")
	// Fila con pocas columnas
	f.WriteString("4,JE000004,malformed\n")

	cfg := loader.Config{NumWorkers: 2, ChunkSize: 10, BufferSize: 4, Verbose: false}
	l := loader.New(cfg)
	records, stats, err := l.LoadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	if stats.ValidRecords != 1 {
		t.Errorf("expected 1 valid record, got %d", stats.ValidRecords)
	}
	if stats.InvalidRecords < 3 {
		t.Errorf("expected >= 3 invalid records, got %d", stats.InvalidRecords)
	}
	if len(records) != 1 {
		t.Errorf("expected 1 record in slice, got %d", len(records))
	}
}

// BenchmarkLoadFile mide el throughput del loader concurrente.
func BenchmarkLoadFile(b *testing.B) {
	path := generateTestCSV(&testing.T{}, 50_000)
	defer os.Remove(path)

	cfg := loader.Config{NumWorkers: 8, ChunkSize: 2000, BufferSize: 32, Verbose: false}
	l := loader.New(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		records, _, err := l.LoadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		b.SetBytes(int64(len(records)))
	}
}

// TestCleanRecord_Fields verifica los campos derivados (Hour, DayOfWeek) de un registro limpio.
func TestCleanRecord_Fields(t *testing.T) {
	path := generateTestCSV(t, 100)
	defer os.Remove(path)

	cfg := loader.Config{NumWorkers: 2, ChunkSize: 50, BufferSize: 4, Verbose: false}
	l := loader.New(cfg)
	records, _, err := l.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range records {
		if r.Hour < 0 || r.Hour > 23 {
			t.Errorf("invalid Hour=%d for record ID=%d", r.Hour, r.ID)
		}
		if r.DayOfWeek < 0 || r.DayOfWeek > 6 {
			t.Errorf("invalid DayOfWeek=%d for record ID=%d", r.DayOfWeek, r.ID)
		}
		if r.Year < 2001 || r.Year > 2030 {
			t.Errorf("invalid Year=%d for record ID=%d", r.Year, r.ID)
		}
		if r.PrimaryType == "" {
			t.Errorf("empty PrimaryType for record ID=%d", r.ID)
		}
		_ = models.CleanRecord{} // ensure import used
		_ = strings.ToUpper("")
		_ = csv.NewReader(nil)
		_ = time.Now()
	}
}
