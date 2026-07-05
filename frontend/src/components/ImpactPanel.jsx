import { useState } from 'react';
import { api, CRIME_TYPES, DISTRICTS } from '../api.js';
import CrimeFilter from './CrimeFilter.jsx';

export default function ImpactPanel() {
  const [district, setDistrict] = useState(11);
  const [crimes, setCrimes] = useState([...CRIME_TYPES]);
  const [curve, setCurve] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const generate = async () => {
    setLoading(true);
    setError('');
    try {
      const results = await Promise.all(
        Array.from({ length: 24 }, async (_, hour) => {
          const rs = await Promise.all(
            crimes.map((t) =>
              api.predict({
                hour,
                district,
                primary_type: t,
                domestic: false,
                day_of_week: 5,
                month: 7,
              })
            )
          );
          return (
            rs.reduce((s, r) => s + r.data.arrest_probability, 0) / rs.length
          );
        })
      );
      setCurve(results.map((p, hour) => ({ hour, p })));
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const districtName = DISTRICTS.find((d) => d.id === district)?.name || district;
  const maxP = curve ? Math.max(...curve.map((c) => c.p), 0.01) : 1;
  // tope del eje: decena superior al pico (9% → 10, 83% → 90, 95% → 100)
  const axisMax = Math.min(100, Math.max(10, Math.ceil((maxP * 100) / 10) * 10));
  const ticks = [0, 0.25, 0.5, 0.75, 1].map((f) => Math.round(axisMax * f));
  const peak = curve
    ? curve.reduce((a, b) => (b.p > a.p ? b : a), curve[0])
    : null;

  return (
    <div className="grid" style={{ gap: 18 }}>
      <div className="card">
        <h2>
          ¿A qué hora es mayor el riesgo?
          <small>curva generada con 24 predicciones reales del clúster</small>
        </h2>
        <div className="grid cols-3" style={{ alignItems: 'end' }}>
          <div>
            <label>Distrito policial</label>
            <select
              value={district}
              onChange={(e) => setDistrict(+e.target.value)}
            >
              {DISTRICTS.map((d) => (
                <option key={d.id} value={d.id}>
                  {d.id} — {d.name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label>Tipos de crimen</label>
            <CrimeFilter selected={crimes} onChange={setCrimes} />
          </div>
          <div>
            <button
              className="btn"
              onClick={generate}
              disabled={loading || crimes.length === 0}
            >
              {loading
                ? `Consultando ${24 * crimes.length} predicciones…`
                : 'Generar curva de riesgo'}
            </button>
          </div>
        </div>
        {error && <div className="error">{error}</div>}

        {curve && (
          <>
            <svg
              viewBox="0 0 760 240"
              style={{ width: '100%', marginTop: 16 }}
              role="img"
            >
              {ticks.map((t) => {
                const y = 190 - (t / axisMax) * 170;
                return (
                  <g key={t}>
                    <line x1="38" y1={y} x2="752" y2={y}
                      stroke="var(--border)" strokeWidth="1"
                      strokeDasharray={t === 0 ? 'none' : '3,4'} />
                    <text x="32" y={y + 3} textAnchor="end" fontSize="9"
                      fill="var(--muted)">{t}%</text>
                  </g>
                );
              })}
              {curve.map(({ hour, p }) => {
                const h = ((p * 100) / axisMax) * 170;
                const color =
                  p >= 0.66 ? 'var(--bad)' : p >= 0.33 ? 'var(--warn)' : 'var(--ok)';
                return (
                  <g key={hour}>
                    <rect
                      x={hour * 29.5 + 44}
                      y={190 - h}
                      width={22}
                      height={h}
                      rx={4}
                      fill={color}
                      opacity={0.85}
                    >
                      <title>{`${hour}:00 → ${(p * 100).toFixed(1)}%`}</title>
                    </rect>
                    <text
                      x={hour * 29.5 + 55}
                      y={208}
                      textAnchor="middle"
                      fontSize="9"
                      fill="var(--muted)"
                    >
                      {hour}
                    </text>
                  </g>
                );
              })}
            </svg>
            <div className="chart-note">
              Probabilidad de arresto por hora del día · distrito {district} (
              {districtName}) ·{' '}
              {crimes.length === CRIME_TYPES.length
                ? 'todos los tipos'
                : crimes.length === 1
                ? crimes[0]
                : `promedio de ${crimes.length} tipos`}
              . Hora pico:{' '}
              <strong style={{ color: 'var(--text)' }}>
                {peak.hour}:00 ({(peak.p * 100).toFixed(1)}%)
              </strong>
              . Verde = 0–33 %, ámbar = 33–66 %, rojo = 66–100 %.
            </div>
          </>
        )}
      </div>

    </div>
  );
}
