import { useState } from 'react';
import { api, CRIME_TYPES, DAYS, MONTHS, DISTRICTS } from '../api.js';
import CrimeFilter from './CrimeFilter.jsx';
import DistrictMap from './DistrictMap.jsx';

export default function PredictPanel() {
  const [form, setForm] = useState({
    hour: new Date().getHours(),
    day_of_week: new Date().getDay(),
    month: new Date().getMonth() + 1,
    domestic: false,
  });
  const [crimes, setCrimes] = useState([...CRIME_TYPES]); // slicer: todos por defecto
  const [results, setResults] = useState(null); // { [district]: respuesta }
  const [selected, setSelected] = useState(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const set = (k, v) => setForm((f) => ({ ...f, [k]: v }));

  const generate = async () => {
    setLoading(true);
    setError('');
    try {
      const responses = await Promise.all(
        DISTRICTS.map(async (d) => {
          const rs = await Promise.all(
            crimes.map((t) =>
              api.predict({ ...form, primary_type: t, district: d.id })
            )
          );
          const avg =
            rs.reduce((s, r) => s + r.data.arrest_probability, 0) / rs.length;
          return [
            d.id,
            {
              arrest_probability: avg,
              used_beat: rs[0].data.used_beat,
              used_community_area: rs[0].data.used_community_area,
            },
          ];
        })
      );
      const map = Object.fromEntries(responses);
      setResults(map);
      // seleccionar automáticamente el distrito de mayor riesgo
      const top = responses.reduce((a, b) =>
        b[1].arrest_probability > a[1].arrest_probability ? b : a
      );
      setSelected(top[0]);
    } catch (err) {
      setError(err.message);
      setResults(null);
    } finally {
      setLoading(false);
    }
  };

  const sel = selected != null && results ? results[selected] : null;
  const selName = DISTRICTS.find((d) => d.id === selected)?.name;
  const top =
    results &&
    Object.entries(results).reduce((a, b) =>
      b[1].arrest_probability > a[1].arrest_probability ? b : a
    );

  let uniformNote = null;
  if (results) {
    const probs = Object.values(results).map((r) => r.arrest_probability);
    const lo = Math.min(...probs);
    const hi = Math.max(...probs);
    if (hi - lo < 0.1) {
      const crimeTxt =
        crimes.length === 1 ? crimes[0] : 'los tipos seleccionados';
      uniformNote =
        hi > 0.8
          ? `Las predicciones son casi uniformes (${(lo * 100).toFixed(0)}–${(hi * 100).toFixed(0)} %) porque ${crimeTxt} ${crimes.length === 1 ? 'es un delito' : 'son delitos'} de descubrimiento: entran al registro casi siempre porque la policía los observó e hizo el arresto en el acto, en cualquier distrito. La geografía influye poco en su desenlace.`
          : `Las predicciones varían poco entre distritos (${(lo * 100).toFixed(0)}–${(hi * 100).toFixed(0)} %): para este escenario, la geografía influye poco en la probabilidad de arresto.`;
    }
  }

  return (
    <div className="predict-layout">
      <div className="predict-side">
        <div className="card">
          <h2>
            Escenario
            <small>una predicción por distrito</small>
          </h2>
          <label>Hora del día</label>
          <select value={form.hour} onChange={(e) => set('hour', +e.target.value)}>
            {Array.from({ length: 24 }, (_, h) => (
              <option key={h} value={h}>
                {String(h).padStart(2, '0')}:00
              </option>
            ))}
          </select>
          <label>Día de la semana</label>
          <select value={form.day_of_week} onChange={(e) => set('day_of_week', +e.target.value)}>
            {DAYS.map((d, i) => (
              <option key={d} value={i}>{d}</option>
            ))}
          </select>
          <label>Mes</label>
          <select value={form.month} onChange={(e) => set('month', +e.target.value)}>
            {MONTHS.map((m, i) => (
              <option key={m} value={i + 1}>{m}</option>
            ))}
          </select>
          <label>Tipos de crimen</label>
          <CrimeFilter selected={crimes} onChange={setCrimes} />
          <label>¿Incidente doméstico?</label>
          <select
            value={form.domestic ? '1' : '0'}
            onChange={(e) => set('domestic', e.target.value === '1')}
          >
            <option value="0">No</option>
            <option value="1">Sí</option>
          </select>
          <button className="btn" onClick={generate} disabled={loading || crimes.length === 0}>
            {loading
              ? `Consultando ${22 * crimes.length} predicciones…`
              : 'Generar mapa de riesgo'}
          </button>
          {error && <div className="error">{error}</div>}
        </div>

        {sel && (
          <div className="card district-detail">
            <h2>
              Distrito {selected} — {selName}
            </h2>
            <div className="detail-prob">
              {(sel.arrest_probability * 100).toFixed(1)} %
            </div>
            <div className="detail-sub">
              probabilidad de arresto
              {crimes.length > 1 ? ` (promedio de ${crimes.length} tipos)` : ''}
            </div>
            <div className="detail-meta">
              predicho con beat <strong>{sel.used_beat}</strong> y área
              comunitaria <strong>{sel.used_community_area}</strong> (los más
              frecuentes del distrito)
            </div>
          </div>
        )}

      </div>

      <div className="predict-main">
        <div className="card predict-map-card">
        <h2>
          Mapa de riesgo por distrito policial
          <small>límites oficiales del CPD · pase el cursor o haga clic</small>
        </h2>
        {!results && (
          <p className="impact-text">
            Configure el escenario y presione{' '}
            <strong>Generar mapa de riesgo</strong>. El sistema realizará 22
            predicciones concurrentes (una por distrito) y coloreará el mapa
            de gris (riesgo bajo) a rojo (riesgo alto).
          </p>
        )}
        <DistrictMap results={results} selected={selected} onSelect={setSelected} />
        {top && (
          <div className="chart-note">
            Mayor riesgo: <strong style={{ color: 'var(--text)' }}>
              Distrito {top[0]} — {DISTRICTS.find((d) => d.id === +top[0])?.name}{' '}
              ({(top[1].arrest_probability * 100).toFixed(1)} %)
            </strong>
          </div>
        )}
          {uniformNote && <div className="uniform-note">ℹ {uniformNote}</div>}
        </div>

        <div className="card model-note">
          <h2>Qué considera el modelo</h2>
          <p>
            El Random Forest estima la probabilidad de que un incidente con
            estas características termine en arresto, según lo aprendido de
            los registros históricos reales. Considera la hora, el día, el
            mes, el tipo de crimen, la índole doméstica y la ubicación
            (distrito, beat y área comunitaria). El beat y el área comunitaria
            se completan automáticamente con los valores más frecuentes de
            cada distrito, para que cada consulta describa un escenario que
            existe en los datos.
          </p>
        </div>
      </div>
    </div>
  );
}
