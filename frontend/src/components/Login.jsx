import { useState } from 'react';
import { api } from '../api.js';

export default function Login({ onLogin }) {
  const [username, setUsername] = useState('admin');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const submit = async (e) => {
    e.preventDefault();
    setError('');
    setLoading(true);
    try {
      const { data } = await api.login(username, password);
      onLogin({ token: data.token, user: data.user, role: data.role });
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="login-wrap">
      <div className="login-box">
        <div className="brand login-brand">
          Chicago<span>Crimes</span> · Predicción de riesgo
        </div>
        <div className="card login-card">
          <h1>Iniciar sesión</h1>
        <form onSubmit={submit}>
          <label>Usuario</label>
          <input
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="username"
          />
          <label>Contraseña</label>
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
          />
          <button className="btn" disabled={loading}>
            {loading ? 'Verificando…' : 'Entrar'}
          </button>
          {error && <div className="error">{error}</div>}
        </form>
        </div>
      </div>
    </div>
  );
}
