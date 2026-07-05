const API_BASE =
  import.meta.env.VITE_API_URL || `http://${window.location.hostname}:8080`;

export const WS_URL = API_BASE.replace(/^http/, 'ws') + '/ws/metrics';

async function request(path, { method = 'GET', body, token } = {}) {
  const headers = { 'Content-Type': 'application/json' };
  if (token) headers['Authorization'] = `Bearer ${token}`;
  const res = await fetch(`${API_BASE}${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });
  const cache = res.headers.get('X-Cache'); // HIT / MISS de Redis
  let data = null;
  try {
    data = await res.json();
  } catch {
    /* respuesta sin cuerpo */
  }
  if (!res.ok) {
    throw new Error(data?.error || `HTTP ${res.status}`);
  }
  return { data, cache };
}

export const api = {
  login: (username, password) =>
    request('/api/login', { method: 'POST', body: { username, password } }),
  predict: (query) =>
    request('/api/predict', { method: 'POST', body: query }),
  train: (token, opts = {}) =>
    request('/api/train', { method: 'POST', body: opts, token }),
  metrics: () => request('/api/metrics'),
  health: () => request('/api/health'),
};

export const CRIME_TYPES = [
  'THEFT',
  'BATTERY',
  'CRIMINAL DAMAGE',
  'NARCOTICS',
  'ASSAULT',
  'BURGLARY',
  'MOTOR VEHICLE THEFT',
  'ROBBERY',
  'DECEPTIVE PRACTICE',
  'CRIMINAL TRESPASS',
  'WEAPONS VIOLATION',
  'PROSTITUTION',
];

export const DISTRICTS = [
  { id: 1, name: 'Central' },
  { id: 2, name: 'Wentworth' },
  { id: 3, name: 'Grand Crossing' },
  { id: 4, name: 'South Chicago' },
  { id: 5, name: 'Calumet' },
  { id: 6, name: 'Gresham' },
  { id: 7, name: 'Englewood' },
  { id: 8, name: 'Chicago Lawn' },
  { id: 9, name: 'Deering' },
  { id: 10, name: 'Ogden' },
  { id: 11, name: 'Harrison' },
  { id: 12, name: 'Near West' },
  { id: 14, name: 'Shakespeare' },
  { id: 15, name: 'Austin' },
  { id: 16, name: 'Jefferson Park' },
  { id: 17, name: 'Albany Park' },
  { id: 18, name: 'Near North' },
  { id: 19, name: 'Town Hall' },
  { id: 20, name: 'Lincoln' },
  { id: 22, name: 'Morgan Park' },
  { id: 24, name: 'Rogers Park' },
  { id: 25, name: 'Grand Central' },
];

export const MONTHS = [
  'Enero', 'Febrero', 'Marzo', 'Abril', 'Mayo', 'Junio',
  'Julio', 'Agosto', 'Septiembre', 'Octubre', 'Noviembre', 'Diciembre',
];

export const DAYS = [
  'Domingo',
  'Lunes',
  'Martes',
  'Miércoles',
  'Jueves',
  'Viernes',
  'Sábado',
];
