import { useEffect, useRef, useState } from 'react';
import { api, WS_URL } from '../api.js';

export default function AdminPanel({ token }) {
  const [m, setM] = useState(null);
  const [wsUp, setWsUp] = useState(false);
  const [training, setTraining] = useState(false);
  const [trainMsg, setTrainMsg] = useState('');
  const wsRef = useRef(null);

  useEffect(() => {
    let alive = true;
    const connect = () => {
      const ws = new WebSocket(WS_URL);
      wsRef.current = ws;
      ws.onopen = () => alive && setWsUp(true);
      ws.onmessage = (ev) => {
        if (!alive) return;
        try {
          setM(JSON.parse(ev.data));
        } catch {
          /* frame ilegible */
        }
      };
      ws.onclose = () => {
        if (!alive) return;
        setWsUp(false);
        setTimeout(connect, 3000); // reconexión automática
      };
      ws.onerror = () => ws.close();
    };
    connect();
    return () => {
      alive = false;
      wsRef.current?.close();
    };
  }, []);

  const retrain = async () => {
    setTraining(true);
    setTrainMsg('');
    try {
      const { data } = await api.train(token, { num_trees: 100 });
      setTrainMsg(
        `Modelo reentrenado: ${data.train_stats.num_trees} árboles en ${data.train_stats.elapsed_ms} ms (${data.node_stats.length} nodos)`
      );
    } catch (err) {
      setTrainMsg(`Error: ${err.message}`);
    } finally {
      setTraining(false);
    }
  };

  if (!m)
    return (
      <div className="card">
        <h2>Panel de administración</h2>
        <p className="impact-text">
          Conectando al canal WebSocket de métricas… ({WS_URL})
        </p>
      </div>
    );

  const hitRate =
    m.cache_hits + m.cache_misses > 0
      ? ((m.cache_hits / (m.cache_hits + m.cache_misses)) * 100).toFixed(1)
      : '0.0';

  return (
    <div className="grid" style={{ gap: 18 }}>
      <div className="ws-status">
        <span className={`dot ${wsUp ? 'up' : 'down'}`} /> WebSocket{' '}
        {wsUp ? 'conectado' : 'reconectando…'} · actualización cada 2 s ·{' '}
        {m.timestamp}
      </div>

      <div className="grid cols-4">
        <div className={`card stat ${m.nodes_alive === m.nodes_total ? 'ok' : 'bad'}`}>
          <div className="value">
            {m.nodes_alive}/{m.nodes_total}
          </div>
          <div className="label">nodos ML activos</div>
        </div>
        <div className="card stat accent">
          <div className="value">{m.model_trees}</div>
          <div className="label">árboles del modelo</div>
        </div>
        <div className="card stat">
          <div className="value">{m.predictions}</div>
          <div className="label">predicciones servidas</div>
        </div>
        <div className="card stat warn">
          <div className="value">{m.avg_latency_ms?.toFixed(2)} ms</div>
          <div className="label">latencia media</div>
        </div>
      </div>

      <div className="grid cols-2">
        <div className="card">
          <h2>Nodos del clúster</h2>
          <table>
            <thead>
              <tr>
                <th>Dirección</th>
                <th>Estado</th>
                <th>Últ. entrenamiento</th>
              </tr>
            </thead>
            <tbody>
              {m.nodes?.map((n) => {
                const st = m.last_node_stats?.find((s) => s.addr === n.addr);
                return (
                  <tr key={n.addr}>
                    <td>{n.addr}</td>
                    <td>
                      <span className={`dot ${n.alive ? 'up' : 'down'}`} />
                      {n.alive ? 'activo' : 'caído'}
                    </td>
                    <td>
                      {st
                        ? `${st.trees} árboles · ${st.elapsed_ms} ms · ${st.workers} workers de entrenamiento`
                        : '—'}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>

        <div className="card">
          <h2>Métricas del modelo (validación)</h2>
          {m.model_metrics ? (
            <table>
              <tbody>
                <tr>
                  <td>Accuracy</td>
                  <td>{(m.model_metrics.accuracy * 100).toFixed(2)}%</td>
                </tr>
                <tr>
                  <td>Precision</td>
                  <td>{(m.model_metrics.precision * 100).toFixed(2)}%</td>
                </tr>
                <tr>
                  <td>Recall</td>
                  <td>{(m.model_metrics.recall * 100).toFixed(2)}%</td>
                </tr>
                <tr>
                  <td>F1-score</td>
                  <td>{(m.model_metrics.f1_score * 100).toFixed(2)}%</td>
                </tr>
                <tr>
                  <td>Último entrenamiento</td>
                  <td>{m.last_train_ms} ms</td>
                </tr>
              </tbody>
            </table>
          ) : (
            <p className="impact-text">El modelo aún no está entrenado.</p>
          )}
          <div style={{ marginTop: 16 }}>
            <button className="btn" onClick={retrain} disabled={training}>
              {training
                ? 'Entrenando en el clúster…'
                : 'Reentrenar modelo (distribuido)'}
            </button>
            {trainMsg && (
              <div className="chart-note" style={{ marginTop: 10 }}>
                {trainMsg}
              </div>
            )}
          </div>
        </div>
      </div>

      <div className="uniform-note">
        ℹ Las <strong>goroutines de la API</strong> son las rutinas vivas del
        proceso de la API (HTTP, WebSocket, métricas), mientras que los{' '}
        <strong>workers</strong> de cada nodo son los pools de entrenamiento
        de procesos independientes del clúster — son contadores de procesos
        distintos y no tienen por qué coincidir.
      </div>

      <div className="grid cols-4">
        <div className="card stat warn">
          <div className="value">{m.cpu_api_percent?.toFixed(1)}%</div>
          <div className="label">CPU total consumida por la API</div>
        </div>
        <div className="card stat">
          <div className="value">{m.memory_mb?.toFixed(1)} MB</div>
          <div className="label">memoria de la API</div>
        </div>
        <div className="card stat">
          <div className="value">{m.goroutines}</div>
          <div className="label">goroutines de la API</div>
        </div>
        <div className="card stat">
          <div className="value">
            {Math.floor(m.uptime_seconds / 60)}m {m.uptime_seconds % 60}s
          </div>
          <div className="label">uptime de la API</div>
        </div>
      </div>

      <div className="grid cols-4">
        <div className="card stat ok">
          <div className="value">{hitRate}%</div>
          <div className="label">
            cache hit rate ({m.cache_hits} HIT / {m.cache_misses} MISS)
          </div>
        </div>
        <div className={`card stat ${m.redis_healthy ? 'ok' : 'bad'}`}>
          <div className="value">{m.redis_healthy ? 'UP' : 'DOWN'}</div>
          <div className="label">Redis (caché)</div>
        </div>
        <div className={`card stat ${m.mongo_healthy ? 'ok' : 'bad'}`}>
          <div className="value">{m.mongo_healthy ? 'UP' : 'DOWN'}</div>
          <div className="label">MongoDB (persistencia)</div>
        </div>
      </div>
    </div>
  );
}
