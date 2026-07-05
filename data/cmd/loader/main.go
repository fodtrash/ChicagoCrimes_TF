// Comando principal del cargador concurrente de crímenes de Chicago.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"crimes-loader/data/internal/loader"
	"crimes-loader/data/internal/pipeline"
)

var driveConfirmTokenPattern = regexp.MustCompile(`confirm=([0-9A-Za-z_\-]+)`)
var driveInputPattern = regexp.MustCompile(`name="([^"]+)" value="([^"]*)"`)
var driveFormActionPattern = regexp.MustCompile(`(?i)<form[^>]*id="download-form"[^>]*action="([^"]+)"`)

func main() {
	// ── Flags de línea de comandos ────────────────────────────────────────────
	fileFlag := flag.String("file", "data/crimes.csv", "Ruta al CSV del dataset Chicago Crimes")
	workersFlag := flag.Int("workers", runtime.NumCPU(), "Número de goroutines trabajadoras")
	chunkFlag := flag.Int("chunk", 5000, "Líneas CSV por bloque de trabajo")
	outputFlag := flag.String("output", "output", "Directorio de salida")
	benchFlag := flag.Bool("benchmark", false, "Ejecuta comparativa secuencial vs concurrente")
	verboseFlag := flag.Bool("verbose", true, "Imprime progreso en tiempo real")
	flag.Parse()

	fmt.Println("╔════════════════════════════════════════════════════╗")
	fmt.Println("║  CC65 — Cargador Concurrente de Datos              ║")
	fmt.Println("║  Dataset: Chicago Crimes 2001-Present              ║")
	fmt.Println("╚════════════════════════════════════════════════════╝")
	fmt.Printf("  CPU cores disponibles: %d\n", runtime.NumCPU())
	fmt.Printf("  Goroutines configuradas: %d\n", *workersFlag)

	csvPath, cleanup, err := prepareCSVSource(*fileFlag)
	if err != nil {
		log.Fatalf("Error preparando el CSV: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	cfg := loader.Config{
		NumWorkers: *workersFlag,
		ChunkSize:  *chunkFlag,
		BufferSize: *workersFlag * 4,
		Verbose:    *verboseFlag,
	}

	if *benchFlag {
		runBenchmark(csvPath, cfg)
		return
	}

	// ── Pipeline normal ──────────────────────────────────────────────────────
	p := pipeline.New(cfg)
	if err := p.Run(csvPath, *outputFlag); err != nil {
		log.Fatalf(" Error en el pipeline: %v", err)
	}
}

func prepareCSVSource(source string) (string, func(), error) {
	if isHTTPSource(source) {
		return downloadCSVSource(source)
	}

	if _, err := os.Stat(source); os.IsNotExist(err) {
		log.Printf("  Archivo no encontrado: %s", source)
		log.Println("   Generando dataset sintético de prueba (100 000 registros)...")
		if err := generateSyntheticCSV(source, 100_000); err != nil {
			return "", nil, fmt.Errorf("error generando dataset de prueba: %w", err)
		}
		log.Printf("   Dataset sintético creado en: %s\n", source)
	}

	return source, nil, nil
}

func isHTTPSource(source string) bool {
	parsed, err := url.Parse(source)
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func downloadCSVSource(source string) (string, func(), error) {
	parsed, err := url.Parse(source)
	if err != nil {
		return "", nil, fmt.Errorf("URL inválida: %w", err)
	}

	tempFile, err := os.CreateTemp("", "crimes_loader_*.csv")
	if err != nil {
		return "", nil, fmt.Errorf("no se pudo crear un archivo temporal: %w", err)
	}
	tempPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return "", nil, fmt.Errorf("no se pudo cerrar el archivo temporal: %w", err)
	}

	cleanup := func() {
		_ = os.Remove(tempPath)
	}

	if strings.Contains(strings.ToLower(parsed.Host), "drive.google.com") {
		if err := downloadGoogleDriveCSV(parsed, tempPath); err != nil {
			cleanup()
			return "", nil, err
		}
		return tempPath, cleanup, nil
	}

	if err := downloadHTTPToFile(source, tempPath); err != nil {
		cleanup()
		return "", nil, err
	}

	return tempPath, cleanup, nil
}

func downloadGoogleDriveCSV(parsed *url.URL, destination string) error {
	fileID, err := extractGoogleDriveFileID(parsed)
	if err != nil {
		return err
	}

	directURL := "https://drive.google.com/uc?export=download&id=" + url.QueryEscape(fileID)
	client, err := newHTTPClient()
	if err != nil {
		return err
	}

	return downloadDriveURL(client, directURL, destination)
}

func extractGoogleDriveFileID(parsed *url.URL) (string, error) {
	path := strings.Trim(parsed.Path, "/")
	segments := strings.Split(path, "/")
	for i := 0; i < len(segments)-1; i++ {
		if segments[i] == "file" && segments[i+1] == "d" && i+2 < len(segments) {
			return segments[i+2], nil
		}
	}

	if fileID := strings.TrimSpace(parsed.Query().Get("id")); fileID != "" {
		return fileID, nil
	}

	return "", fmt.Errorf("no se pudo extraer el ID del archivo de Google Drive")
}

func newHTTPClient() (*http.Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear el manejador de cookies: %w", err)
	}
	return &http.Client{Jar: jar}, nil
}

func downloadDriveURL(client *http.Client, downloadURL, destination string) error {
	resp, err := doGET(client, downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if token, ok := driveConfirmToken(resp); ok {
		resp.Body.Close()
		confirmedURL := downloadURL + "&confirm=" + url.QueryEscape(token)
		resp, err = doGET(client, confirmedURL)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("Google Drive devolvió %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/html") {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("no se pudo leer la respuesta de Google Drive: %w", err)
		}
		actionURL, queryParams, err := parseGoogleDriveDownloadForm(string(body))
		if err != nil {
			return fmt.Errorf("Google Drive devolvió una página HTML en lugar del CSV; verifica que el archivo sea público: %w", err)
		}

		reqURL, err := url.Parse(actionURL)
		if err != nil {
			return fmt.Errorf("no se pudo interpretar la URL de descarga de Google Drive: %w", err)
		}

		values := reqURL.Query()
		for key, value := range queryParams {
			values.Set(key, value)
		}
		reqURL.RawQuery = values.Encode()

		resp.Body.Close()
		resp, err = doGET(client, reqURL.String())
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			return fmt.Errorf("Google Drive devolvió %s: %s", resp.Status, strings.TrimSpace(string(body)))
		}
	}

	return writeBodyToFile(resp.Body, destination)
}

func driveConfirmToken(resp *http.Response) (string, bool) {
	for _, cookie := range resp.Cookies() {
		if strings.HasPrefix(cookie.Name, "download_warning") && cookie.Value != "" {
			return cookie.Value, true
		}
	}
	return "", false
}

func extractConfirmToken(body string) string {
	match := driveConfirmTokenPattern.FindStringSubmatch(body)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func parseGoogleDriveDownloadForm(body string) (string, map[string]string, error) {
	actionMatch := driveFormActionPattern.FindStringSubmatch(body)
	if len(actionMatch) != 2 {
		return "", nil, fmt.Errorf("no se encontró el formulario de descarga")
	}

	params := make(map[string]string)
	for _, match := range driveInputPattern.FindAllStringSubmatch(body, -1) {
		if len(match) != 3 {
			continue
		}
		params[match[1]] = htmlUnescape(match[2])
	}

	if params["id"] == "" || params["export"] == "" {
		return "", nil, fmt.Errorf("el formulario de Google Drive no incluyó los campos necesarios")
	}

	return htmlUnescape(actionMatch[1]), params, nil
}

func htmlUnescape(value string) string {
	value = strings.ReplaceAll(value, "&amp;", "&")
	value = strings.ReplaceAll(value, "&quot;", `"`)
	value = strings.ReplaceAll(value, "&#39;", "'")
	value = strings.ReplaceAll(value, "&lt;", "<")
	value = strings.ReplaceAll(value, "&gt;", ">")
	return value
}

func downloadHTTPToFile(sourceURL, destination string) error {
	client, err := newHTTPClient()
	if err != nil {
		return err
	}

	resp, err := doGET(client, sourceURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("descarga fallida (%s): %s", resp.Status, strings.TrimSpace(string(body)))
	}

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/html") {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if err != nil {
			return fmt.Errorf("no se pudo leer la respuesta remota: %w", err)
		}
		return fmt.Errorf("la URL devolvió HTML en lugar de CSV: %s", strings.TrimSpace(string(body)))
	}

	return writeBodyToFile(resp.Body, destination)
}

func doGET(client *http.Client, sourceURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, fmt.Errorf("no se pudo crear la petición HTTP: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/126.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("no se pudo descargar el archivo: %w", err)
	}
	return resp, nil
}

func writeBodyToFile(body io.Reader, destination string) error {
	out, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("no se pudo crear el archivo destino: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, body); err != nil {
		return fmt.Errorf("no se pudo escribir el CSV descargado: %w", err)
	}
	return nil
}

// runBenchmark compara el rendimiento entre 1 worker (pseudo-secuencial)
// y N workers (concurrente) para demostrar el speedup obtenido.
func runBenchmark(csvPath string, cfg loader.Config) {
	fmt.Println("\n  MODO BENCHMARK: Comparativa secuencial vs. concurrente")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	workerCounts := []int{1, 2, 4, cfg.NumWorkers}
	type result struct {
		workers int
		elapsed time.Duration
		records int64
	}
	results := make([]result, 0, len(workerCounts))

	for _, nw := range workerCounts {
		c := cfg
		c.NumWorkers = nw
		c.Verbose = false
		l := loader.New(c)

		fmt.Printf("  Ejecutando con %2d worker(s)... ", nw)
		start := time.Now()
		records, stats, err := l.LoadFile(csvPath)
		elapsed := time.Since(start)

		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			continue
		}
		_ = records
		results = append(results, result{workers: nw, elapsed: elapsed, records: stats.ValidRecords})
		fmt.Printf("✓ %d registros en %v\n", stats.ValidRecords, elapsed.Round(time.Millisecond))
	}

	if len(results) == 0 {
		return
	}

	fmt.Println("\n  Resultados de speedup:")
	fmt.Println("  ┌─────────┬─────────────┬──────────┐")
	fmt.Println("  │ Workers │ Tiempo      │ Speedup  │")
	fmt.Println("  ├─────────┼─────────────┼──────────┤")
	baseline := results[0].elapsed
	for _, r := range results {
		speedup := float64(baseline) / float64(r.elapsed)
		fmt.Printf("  │ %7d │ %11v │ %7.2fx │\n", r.workers, r.elapsed.Round(time.Millisecond), speedup)
	}
	fmt.Println("  └─────────┴─────────────┴──────────┘")
}
