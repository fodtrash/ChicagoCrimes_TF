package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"
)

// Datos representativos del dataset real de Chicago Crimes
var (
	crimeTypes = []string{
		"THEFT", "BATTERY", "CRIMINAL DAMAGE", "NARCOTICS", "ASSAULT",
		"BURGLARY", "MOTOR VEHICLE THEFT", "ROBBERY", "DECEPTIVE PRACTICE",
		"OTHER OFFENSE", "CRIMINAL TRESPASS", "WEAPONS VIOLATION",
		"PUBLIC PEACE VIOLATION", "HOMICIDE", "ARSON",
	}

	locationDescs = []string{
		"STREET", "RESIDENCE", "APARTMENT", "SIDEWALK", "PARKING LOT",
		"ALLEY", "RESTAURANT", "GAS STATION", "CONVENIENCE STORE",
		"SCHOOL", "PARK", "CTA PLATFORM", "HOTEL/MOTEL",
	}

	districts = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 14, 15, 16, 17, 18, 19, 20, 22, 24, 25}
	// Coordenadas aproximadas de Chicago
	latMin, latMax = 41.65, 42.02
	lonMin, lonMax = -87.87, -87.52
)

// generateSyntheticCSV genera un archivo CSV sintético con n registros
// siguiendo el esquema del Chicago Crimes dataset.
func generateSyntheticCSV(path string, n int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Encabezado exacto del Chicago Data Portal
	header := "ID,Case Number,Date,Block,IUCR,Primary Type,Description,Location Description," +
		"Arrest,Domestic,Beat,District,Ward,Community Area,FBI Code,X Coordinate," +
		"Y Coordinate,Year,Updated On,Latitude,Longitude,Location\n"
	if _, err := f.WriteString(header); err != nil {
		return err
	}

	// Fechas de ejemplo: 2015-2023
	baseYear := 2015
	for i := 1; i <= n; i++ {
		year := baseYear + rng.Intn(9)
		month := 1 + rng.Intn(12)
		day := 1 + rng.Intn(28)
		hour := rng.Intn(24)
		minute := rng.Intn(60)
		second := rng.Intn(60)
		ampm := "AM"
		displayHour := hour
		if hour >= 12 {
			ampm = "PM"
			if hour > 12 {
				displayHour = hour - 12
			}
		}
		if displayHour == 0 {
			displayHour = 12
		}

		date := fmt.Sprintf("%02d/%02d/%d %02d:%02d:%02d %s",
			month, day, year, displayHour, minute, second, ampm)

		district := districts[rng.Intn(len(districts))]
		beat := district*100 + rng.Intn(10) + 1
		ward := 1 + rng.Intn(50)
		communityArea := 1 + rng.Intn(77)
		crimeType := crimeTypes[rng.Intn(len(crimeTypes))]
		locDesc := locationDescs[rng.Intn(len(locationDescs))]
		arrest := "false"
		if rng.Float32() < 0.25 {
			arrest = "true"
		}
		domestic := "false"
		if rng.Float32() < 0.12 {
			domestic = "true"
		}

		lat := latMin + rng.Float64()*(latMax-latMin)
		lon := lonMin + rng.Float64()*(lonMax-lonMin)

		caseNum := fmt.Sprintf("JE%06d", i)
		iucr := fmt.Sprintf("%04d", 1000+rng.Intn(9000))
		fbiCode := fmt.Sprintf("%02d", rng.Intn(30))
		xCoord := 1100000 + rng.Intn(50000)
		yCoord := 1800000 + rng.Intn(100000)
		updatedOn := fmt.Sprintf("%02d/%02d/%d %02d:%02d:%02d AM",
			month, day, year+1, hour%12+1, minute, second)

		line := fmt.Sprintf("%d,%s,%s,0%d N %s ST,%s,%s,SIMPLE,%s,%s,%s,%d,%d,%d,%d,%s,%d,%d,%d,%s,%.6f,%.6f,\"(%f %f)\"\n",
			i, caseNum, date, 1+rng.Intn(999), "MAIN",
			iucr, crimeType, locDesc,
			arrest, domestic,
			beat, district, ward, communityArea,
			fbiCode, xCoord, yCoord, year, updatedOn,
			lat, lon, lat, lon)

		if _, err := f.WriteString(line); err != nil {
			return err
		}
	}
	return nil
}
