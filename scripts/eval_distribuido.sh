#!/bin/bash
# ══════════════════════════════════════════════════════════════════════
# Evaluación experimental del sistema (sección 11 del informe)
#
#   1. Speedup del entrenamiento distribuido con 1, 2 y 3 nodos
#   2. Latencia de POST /api/predict (p50 / p95 / máx) bajo carga
#   3. Eficacia de la caché Redis (hit rate y aceleración HIT vs MISS)
#
# Requiere: binarios compilados (go build) y Redis corriendo.
#   cd ml && go build -o ../scripts/bin/node ./cmd/node \
#         && go build -o ../scripts/bin/api  ./cmd/api
#   bash scripts/eval_distribuido.sh
# ══════════════════════════════════════════════════════════════════════
set -u
BIN="${BIN:-$(dirname "$0")/bin}"
DATA="${DATA:-$(dirname "$0")/../data/output/clean_records_sample.json}"
TREES="${TREES:-100}"
API_PORT=8090

cleanup() { pkill -f "$BIN/node" 2>/dev/null; pkill -f "$BIN/api" 2>/dev/null; }
trap cleanup EXIT
command -v python3 >/dev/null || { echo "se requiere python3"; exit 1; }

login_and_train() { # $1 = lista de nodos
  TOKEN=$(curl -s -X POST localhost:$API_PORT/api/login -H "Content-Type: application/json" \
    -d '{"username":"admin","password":"admin123"}' | python3 -c "import sys,json;print(json.load(sys.stdin)['token'])")
  curl -s -X POST localhost:$API_PORT/api/train -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" -d "{\"num_trees\":$TREES}"
}

echo "════════════════════════════════════════════════════"
echo " 1) SPEEDUP DEL ENTRENAMIENTO DISTRIBUIDO ($TREES árboles)"
echo "════════════════════════════════════════════════════"
declare -A RESULTS
for N in 1 2 3; do
  cleanup; sleep 0.5
  NODES=""
  for i in $(seq 1 $N); do
    "$BIN/node" -port $((9100+i)) > /dev/null 2>&1 &
    NODES="${NODES}localhost:$((9100+i)),"
  done
  NODES=${NODES%,}
  DATA_FILE="$DATA" "$BIN/api" -port $API_PORT -nodes "$NODES" > /dev/null 2>&1 &
  sleep 2
  MS=$(login_and_train "$NODES" | python3 -c "import sys,json;print(json.load(sys.stdin)['train_stats']['elapsed_ms'])")
  RESULTS[$N]=$MS
  echo "  $N nodo(s): ${MS} ms"
done
python3 - "${RESULTS[1]}" "${RESULTS[2]}" "${RESULTS[3]}" <<'PYEOF'
import sys
t1, t2, t3 = map(float, sys.argv[1:4])
print(f"\n  | Nodos | Tiempo (ms) | Speedup | Eficiencia |")
print(f"  |-------|-------------|---------|------------|")
for n, t in [(1, t1), (2, t2), (3, t3)]:
    s = t1 / t
    print(f"  | {n}     | {t:>11.0f} | {s:>6.2f}x | {s/n*100:>9.1f}% |")
PYEOF

echo
echo "════════════════════════════════════════════════════"
echo " 2) LATENCIA DE /api/predict (200 consultas únicas, 16 hilos)"
echo "════════════════════════════════════════════════════"
python3 - <<PYEOF
import json, time, random, urllib.request, concurrent.futures, statistics
URL = "http://localhost:$API_PORT/api/predict"
TYPES = ["THEFT","BATTERY","NARCOTICS","ASSAULT","BURGLARY","ROBBERY"]
def one(i):
    body = json.dumps({"hour": i % 24, "district": i % 25 + 1,
        "primary_type": TYPES[i % len(TYPES)], "domestic": i % 2 == 0,
        "day_of_week": i % 7, "month": i % 12 + 1,
        "community_area": i % 77 + 1, "beat": 100 + i}).encode()
    req = urllib.request.Request(URL, data=body, headers={"Content-Type": "application/json"})
    t0 = time.perf_counter()
    with urllib.request.urlopen(req, timeout=5) as r:
        r.read()
        cache = r.headers.get("X-Cache")
    return (time.perf_counter() - t0) * 1000, cache
with concurrent.futures.ThreadPoolExecutor(16) as ex:
    res = list(ex.map(one, range(200)))
lat = sorted(l for l, _ in res)
print(f"  p50 = {statistics.median(lat):.2f} ms | p95 = {lat[int(len(lat)*0.95)]:.2f} ms | máx = {lat[-1]:.2f} ms")
print(f"  requisito del enunciado: < 100 ms → {'CUMPLE' if lat[int(len(lat)*0.95)] < 100 else 'NO CUMPLE'}")
PYEOF

echo
echo "════════════════════════════════════════════════════"
echo " 3) EFICACIA DE LA CACHÉ REDIS (misma consulta x50)"
echo "════════════════════════════════════════════════════"
python3 - <<PYEOF
import json, time, urllib.request, statistics
URL = "http://localhost:$API_PORT/api/predict"
body = json.dumps({"hour":22,"district":11,"primary_type":"NARCOTICS","domestic":False,
    "day_of_week":5,"month":7,"community_area":25,"beat":90001}).encode()
def one():
    req = urllib.request.Request(URL, data=body, headers={"Content-Type":"application/json"})
    t0 = time.perf_counter()
    with urllib.request.urlopen(req, timeout=5) as r:
        r.read(); c = r.headers.get("X-Cache")
    return (time.perf_counter()-t0)*1000, c
miss_t, miss_c = one()
hits = [one() for _ in range(50)]
hit_lat = [t for t, c in hits if c == "HIT"]
rate = len(hit_lat) / len(hits) * 100
print(f"  1ra consulta (X-Cache: {miss_c}): {miss_t:.2f} ms")
if hit_lat:
    m = statistics.mean(hit_lat)
    print(f"  50 repeticiones → hit rate: {rate:.0f}% | latencia media HIT: {m:.2f} ms")
    print(f"  aceleración de la caché: {miss_t/m:.1f}x")
else:
    print("  ADVERTENCIA: sin HITs — ¿Redis está corriendo? (REDIS_ADDR)")
PYEOF
echo
echo "Listo. Copiar las tablas a la sección 11 del informe."
