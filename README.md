# Sistema distribuido de predicción de riesgo (Chicago Crimes)

Solución del TF de CC65: carga concurrente, entrenamiento distribuido de un
Random Forest en un clúster de nodos Go (TCP + gob), API REST/WebSocket
coordinadora, MongoDB + Redis, todo desplegado con Docker Compose.

## Arquitectura

```
Usuario ⇄ [Frontend SPA]* ⇄ REST/JSON + WebSocket ⇄ [API :8080]
                                                       │  ├─ TCP(gob) ⇄ node1 :9000 ─┐
                                                       │  ├─ TCP(gob) ⇄ node2 :9000 ─┼─ Clúster ML
                                                       │  └─ TCP(gob) ⇄ node3 :9000 ─┘
                                                       ├─ SQL/BSON ⇄ MongoDB :27017 (persistencia)
                                                       └─ RESP     ⇄ Redis   :6379  (caché TTL 10 min)
(*) frontend: React + Vite servido por nginx en el puerto 3000
```

- `data/`: **Parte A** — carga, limpieza y análisis concurrente del CSV (goroutines + channels).
- `ml/`: **Partes B y C** — módulo con tres ejecutables:
  - `cmd/trainer`: entrenamiento local paralelo (Entregable 1).
  - `cmd/node`: nodo trabajador del clúster; recibe `TrainTask` por TCP, entrena su lote con goroutines y devuelve `TrainResult` (codificación `encoding/gob`).
  - `cmd/api`: coordinador del clúster + servicio REST + WebSocket + JWT + Mongo/Redis.

## Frontend (Parte D)

SPA en React (Vite) con cuatro módulos, en `frontend/`:

1. **Login** — autenticación JWT contra `POST /api/login`.
2. **Predicción** — formulario de consulta al modelo; muestra probabilidad,
   nivel de riesgo, latencia y si la respuesta vino de Redis (`X-Cache`).
3. **Impacto social** — curva de riesgo por hora del día generada con 24
   predicciones concurrentes reales al clúster, por distrito y tipo de crimen.
4. **Panel admin** — métricas del clúster en tiempo real vía WebSocket
   (`/ws/metrics`, un frame cada 2 s, con reconexión automática) y botón de
   reentrenamiento distribuido.

Disponible en `http://localhost:3000` al levantar el stack. Para desarrollo:
`cd frontend && npm install && npm run dev`.

## Mapa de distritos (GeoJSON)

El mapa de calor de la pestaña Predicción usa los límites oficiales de los
distritos policiales, del dataset "Boundaries - Police Districts (current)"
del portal de datos de Chicago:
`https://data.cityofchicago.org/Public-Safety/Boundaries-Police-Districts-current-/fthy-xz3r`
(Export → GeoJSON). En el repo: `data/geo/police_districts_full.geojson`
(original) y `frontend/src/data/police_districts.json` (simplificado con
mapshaper al 10 %, 52 KB, para el bundle de la SPA).

## Ejecutar todo el sistema

```bash
docker compose up --build
```

Esto levanta: loader (una vez) → 3 nodos ML → MongoDB → Redis → API.
Con `AUTO_TRAIN=true` la API entrena el modelo distribuido al arrancar.

Para usar el CSV compartido en Google Drive, exporta el enlace como `DATA_FILE`
antes de levantar Compose:

```powershell
$env:DATA_FILE = "https://drive.google.com/file/d/1fUh37cDvjSpB0GGpoSqxPQUyGvY3FDbv/view?usp=sharing"
docker compose up --build
```

Entrenamiento local del Entregable 1 (opcional):

```bash
docker compose --profile local-train up trainer
```

## Endpoints de la API (puerto 8080)

| Método y ruta | Descripción |
| --- | --- |
| `POST /api/login` | Devuelve un JWT (HS256). Credenciales por defecto: `admin/admin123`. |
| `POST /api/train` | (JWT requerido) Dispara el entrenamiento distribuido y persiste metadatos en Mongo. Body opcional: `{"num_trees":100,"max_depth":10}` |
| `POST /api/predict` | Predicción de probabilidad de arresto + nivel de riesgo. Cabecera `X-Cache: HIT/MISS` evidencia la caché Redis. |
| `GET /api/metrics` | Métricas del clúster: nodos vivos, latencia media, throughput del último entrenamiento, estado Mongo/Redis. |
| `GET /api/health` | Sonda de salud usada por el healthcheck de Compose. |
| `GET /api/predictions/recent` | Últimas predicciones persistidas en MongoDB. |
| `GET /ws/metrics` | WebSocket: métricas del clúster en tiempo real (cada 2 s) para el panel admin. |

### Ejemplo de uso

```bash
TOKEN=$(curl -s -X POST localhost:8080/api/login \
  -d '{"username":"admin","password":"admin123"}' | jq -r .token)

curl -X POST localhost:8080/api/train -H "Authorization: Bearer $TOKEN"

curl -i -X POST localhost:8080/api/predict -H "Content-Type: application/json" \
  -d '{"hour":22,"district":11,"primary_type":"NARCOTICS","domestic":false,
       "day_of_week":5,"month":7,"community_area":25,"beat":1131}'
# → {"arrest_probability":0.43,"risk_level":"medio",...}  X-Cache: MISS
# la misma consulta repetida dentro de 10 min → X-Cache: HIT
```

## Verificación de condiciones de carrera

```bash
cd ml
go build -race ./cmd/node && go build -race ./cmd/api
# ejecutar nodos y API con los binarios instrumentados y disparar
# entrenamientos + predicciones concurrentes: 0 WARNING: DATA RACE
```

## Estructura esperada de datos

- `data/crimes.csv`: dataset fuente (si no existe, el loader genera uno sintético).
- `data/output/clean_records_sample.json`: salida del loader, entrada del clúster.
- `data/output/train_results.json`: resultados del entrenamiento local.

## Notas

- El módulo `ml/` usa los drivers oficiales `go-redis` y `mongo-driver`; el
  `go.sum` se genera en el primer build (`go mod tidy` dentro del Dockerfile).
- Si Mongo o Redis no están disponibles, la API arranca igual en modo
  degradado (sin persistencia / sin caché) registrando una advertencia.
