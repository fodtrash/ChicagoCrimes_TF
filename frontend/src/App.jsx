import { useState } from 'react';
import Login from './components/Login.jsx';
import PredictPanel from './components/PredictPanel.jsx';
import ImpactPanel from './components/ImpactPanel.jsx';
import AdminPanel from './components/AdminPanel.jsx';

export default function App() {
  const [session, setSession] = useState(() => {
    const saved = sessionStorage.getItem('cc65_session');
    return saved ? JSON.parse(saved) : null;
  });
  const [tab, setTab] = useState('predict');

  const onLogin = (s) => {
    sessionStorage.setItem('cc65_session', JSON.stringify(s));
    setTab('predict'); // siempre aterrizar en Predicción
    setSession(s);
  };
  const onLogout = () => {
    sessionStorage.removeItem('cc65_session');
    setTab('predict'); // evitar heredar la pestaña de la sesión anterior
    setSession(null);
  };

  if (!session) return <Login onLogin={onLogin} />;

  const isAdmin = session.role === 'admin';
  // Si por cualquier motivo la pestaña activa no está disponible para el
  // rol (p. ej. estado heredado), se cae a Predicción: nunca pantalla vacía.
  const activeTab = tab === 'admin' && !isAdmin ? 'predict' : tab;

  return (
    <>
      <header className="topbar">
        <div className="brand">
          Chicago<span>Crimes</span> · Predicción de riesgo
        </div>
        <nav>
          <button
            className={activeTab === 'predict' ? 'active' : ''}
            onClick={() => setTab('predict')}
          >
            Predicción
          </button>
          <button
            className={activeTab === 'impact' ? 'active' : ''}
            onClick={() => setTab('impact')}
          >
            Impacto social
          </button>
          {isAdmin && (
            <button
              className={activeTab === 'admin' ? 'active' : ''}
              onClick={() => setTab('admin')}
            >
              Panel admin
            </button>
          )}
        </nav>
        <div className="user">
          <span>
            👤 {session.user} · {session.role}
          </span>
          <button onClick={onLogout}>Salir</button>
        </div>
      </header>
      <main>
        {activeTab === 'predict' && <PredictPanel />}
        {activeTab === 'impact' && <ImpactPanel />}
        {activeTab === 'admin' && isAdmin && <AdminPanel token={session.token} />}
      </main>
    </>
  );
}
