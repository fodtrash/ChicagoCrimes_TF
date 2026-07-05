import { useEffect, useRef, useState } from 'react';
import { CRIME_TYPES } from '../api.js';

export default function CrimeFilter({ selected, onChange }) {
  const [open, setOpen] = useState(false);
  const ref = useRef(null);

  useEffect(() => {
    const onDocClick = (e) => {
      if (ref.current && !ref.current.contains(e.target)) setOpen(false);
    };
    document.addEventListener('mousedown', onDocClick);
    return () => document.removeEventListener('mousedown', onDocClick);
  }, []);

  const all = selected.length === CRIME_TYPES.length;
  const none = selected.length === 0;

  const summary = none
    ? 'Ninguno seleccionado'
    : all
    ? 'Todos los tipos'
    : selected.length === 1
    ? selected[0]
    : `${selected.length} tipos seleccionados`;

  const toggle = (t) =>
    onChange(
      selected.includes(t) ? selected.filter((x) => x !== t) : [...selected, t]
    );

  const toggleAll = () => onChange(all ? [] : [...CRIME_TYPES]);

  return (
    <div className="crime-filter" ref={ref}>
      <button
        type="button"
        className="crime-toggle"
        onClick={() => setOpen((o) => !o)}
      >
        <span>{summary}</span>
        <span className="chev">{open ? '▴' : '▾'}</span>
      </button>
      {open && (
        <div className="crime-pop">
          <label className="crime-item select-all">
            <input
              type="checkbox"
              checked={all}
              ref={(el) => el && (el.indeterminate = !all && !none)}
              onChange={toggleAll}
            />
            (Seleccionar todo)
          </label>
          <div className="crime-list">
            {CRIME_TYPES.map((t) => (
              <label key={t} className="crime-item">
                <input
                  type="checkbox"
                  checked={selected.includes(t)}
                  onChange={() => toggle(t)}
                />
                {t}
              </label>
            ))}
          </div>
          <div className="crime-count">
            {none ? 'Seleccione al menos un tipo' : summary}
          </div>
        </div>
      )}
    </div>
  );
}
