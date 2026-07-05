import { useMemo, useState } from 'react';
import districtsGeo from '../data/police_districts.json';
import { DISTRICTS } from '../api.js';

const W = 520;
const H = 640;
const PAD = 12;

// ── Proyección equirrectangular ajustada a la latitud de Chicago ─────
function buildPaths(geo) {
  let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
  const k = Math.cos((41.85 * Math.PI) / 180); // corrección lon→x
  const pts = [];
  for (const f of geo.features) {
    const polys =
      f.geometry.type === 'Polygon'
        ? [f.geometry.coordinates]
        : f.geometry.coordinates;
    for (const poly of polys)
      for (const ring of poly)
        for (const [lon, lat] of ring) {
          const x = lon * k, y = -lat;
          if (x < minX) minX = x;
          if (x > maxX) maxX = x;
          if (y < minY) minY = y;
          if (y > maxY) maxY = y;
        }
  }
  const scale = Math.min((W - 2 * PAD) / (maxX - minX), (H - 2 * PAD) / (maxY - minY));
  const px = (lon) => PAD + (lon * k - minX) * scale;
  const py = (lat) => PAD + (-lat - minY) * scale;

  for (const f of geo.features) {
    const num = parseInt(f.properties.dist_num, 10);
    const polys =
      f.geometry.type === 'Polygon'
        ? [f.geometry.coordinates]
        : f.geometry.coordinates;
    let d = '';
    for (const poly of polys)
      for (const ring of poly) {
        d += ring
          .map(([lon, lat], i) => `${i ? 'L' : 'M'}${px(lon).toFixed(1)},${py(lat).toFixed(1)}`)
          .join('');
        d += 'Z';
      }
    pts.push({ num, d });
  }
  return pts;
}

// ── Escala de color: gris neutro → amarillo → rojo ───────────────────
const STOPS = [
  [209, 213, 219], // #d1d5db gris neutro (0 %)
  [245, 185, 68],  // #f5b944 amarillo   (50 %)
  [225, 29, 72],   // #e11d48 rojo       (100 %)
];

export function riskColor(p) {
  const t = Math.max(0, Math.min(1, p));
  const [a, b, f] = t <= 0.5 ? [STOPS[0], STOPS[1], t * 2] : [STOPS[1], STOPS[2], (t - 0.5) * 2];
  const c = a.map((v, i) => Math.round(v + (b[i] - v) * f));
  return `rgb(${c[0]},${c[1]},${c[2]})`;
}

export default function DistrictMap({ results, onSelect, selected }) {
  const paths = useMemo(() => buildPaths(districtsGeo), []);
  const [hover, setHover] = useState(null);

  const name = (num) => DISTRICTS.find((d) => d.id === num)?.name || `Distrito ${num}`;

  return (
    <div className="map-wrap">
      <svg viewBox={`0 0 ${W} ${H}`} className="district-map" role="img">
        {paths.map(({ num, d }) => {
          const r = results?.[num];
          const fill = r ? riskColor(r.arrest_probability) : 'var(--panel-2)';
          const active = selected === num || hover === num;
          return (
            <path
              key={num}
              d={d}
              fill={fill}
              stroke={active ? '#fff' : 'var(--bg)'}
              strokeWidth={active ? 2 : 0.8}
              style={{ cursor: 'pointer', transition: 'fill .3s' }}
              onMouseEnter={() => setHover(num)}
              onMouseLeave={() => setHover(null)}
              onClick={() => onSelect?.(num)}
            >
              <title>
                {`Distrito ${num} — ${name(num)}`}
                {r ? `\n${(r.arrest_probability * 100).toFixed(1)} % de probabilidad de arresto\nbeat ${r.used_beat} · área comunitaria ${r.used_community_area}` : ''}
              </title>
            </path>
          );
        })}
      </svg>
      {/* Leyenda de gradiente 0–100 % */}
      <div className="map-legend">
        <span>0 %</span>
        <div className="legend-bar" />
        <span>100 %</span>
      </div>
    </div>
  );
}
