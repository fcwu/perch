import { useEffect, useState } from 'react'
import TerminalApp from './TerminalApp'
import ChatApp from './ChatApp'
import AdminRealtimePage from './AdminRealtimePage'
import AdminHistoryPage from './AdminHistoryPage'
import AdminAnalyticsPage from './AdminAnalyticsPage'

interface AuthStatus {
  authenticated: boolean
  username: string
  role: string        // "admin" | "user" | ""
  mode: string        // "single" | "multi"
  auth_method: string // "none" | "password" | "mtls" | "gitlab"
}

// Shared logout button style
function LogoutButton() {
  return (
    <button
      onClick={() => { window.location.href = '/auth/logout' }}
      style={{
        background: 'none', border: '1px solid #444', borderRadius: 4,
        color: '#888', padding: '4px 10px', fontSize: 12, cursor: 'pointer',
        fontFamily: 'monospace',
      }}
    >
      Logout
    </button>
  )
}

// Login screen for multi-user unauthenticated state
function GitLabLoginScreen({ error }: { error: boolean }) {
  return (
    <div style={{
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      height: '100dvh', background: '#1a1a1a', color: '#e0e0e0',
    }}>
      <div style={{
        display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 20,
        background: '#111', padding: 48, borderRadius: 12,
        border: '1px solid #333', minWidth: 320,
      }}>
        <div style={{ fontSize: 24, fontFamily: 'monospace', color: '#4a9eff' }}>Perch</div>
        {error && (
          <div style={{
            color: '#f66', fontSize: 13, textAlign: 'center', padding: '8px 16px',
            background: '#2a0000', borderRadius: 6, border: '1px solid #800',
          }}>
            Access denied. Contact the administrator.
          </div>
        )}
        <button
          onClick={() => { window.location.href = '/auth/gitlab' }}
          style={{
            background: '#fc6d26', border: 'none', borderRadius: 6,
            color: '#fff', padding: '12px 24px', fontSize: 15,
            cursor: 'pointer', fontFamily: 'sans-serif', fontWeight: 600,
            width: '100%',
          }}
        >
          Login with GitLab
        </button>
      </div>
    </div>
  )
}

// Password login overlay (single-user password mode)
function LoginOverlay({ onLogin }: { onLogin: () => void }) {
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    setError('')
    const r = await fetch('/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ password }),
    })
    setLoading(false)
    if (r.ok) {
      onLogin()
    } else {
      setError('Wrong password')
    }
  }

  return (
    <div style={{
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      height: '100dvh', background: '#000',
    }}>
      <form onSubmit={submit} style={{
        display: 'flex', flexDirection: 'column', gap: 12,
        background: '#111', padding: 32, borderRadius: 8, minWidth: 280,
      }}>
        <div style={{ color: '#fff', fontSize: 18, fontFamily: 'monospace', marginBottom: 8 }}>Perch</div>
        <input
          type="password"
          placeholder="Password"
          value={password}
          onChange={e => setPassword(e.target.value)}
          autoFocus
          style={{
            background: '#222', color: '#fff', border: '1px solid #444',
            borderRadius: 4, padding: '8px 12px', fontSize: 14, fontFamily: 'monospace',
          }}
        />
        {error && <div style={{ color: '#f66', fontSize: 12 }}>{error}</div>}
        <button type="submit" disabled={loading} style={{
          background: '#4af', color: '#000', border: 'none', borderRadius: 4,
          padding: '8px 12px', fontSize: 14, fontFamily: 'monospace', cursor: 'pointer',
        }}>
          {loading ? '…' : 'Login'}
        </button>
      </form>
    </div>
  )
}

function TerminalRoot({ status, accessDenied }: { status: AuthStatus, accessDenied: boolean }) {
  const [authenticated, setAuthenticated] = useState(status.authenticated)

  if (!authenticated) {
    if (status.auth_method === 'gitlab') {
      return <GitLabLoginScreen error={accessDenied} />
    }
    // Single-user password mode → show password overlay
    return <LoginOverlay onLogin={() => setAuthenticated(true)} />
  }

  return <TerminalApp showLogout={status.auth_method !== 'none'} />
}

function AdminLoginPage() {
  const [token, setToken] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    setError('')
    const r = await fetch('/admin/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token }),
    })
    setLoading(false)
    if (r.ok) {
      window.location.href = '/admin'
    } else {
      setError('Invalid token')
    }
  }

  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100dvh', background: '#000' }}>
      <form onSubmit={submit} style={{ display: 'flex', flexDirection: 'column', gap: 12, background: '#111', padding: 32, borderRadius: 8, minWidth: 280 }}>
        <div style={{ color: '#fff', fontSize: 18, fontFamily: 'monospace', marginBottom: 8 }}>Admin Login</div>
        <input type="password" placeholder="Admin token" value={token} onChange={e => setToken(e.target.value)} autoFocus
          style={{ background: '#222', color: '#fff', border: '1px solid #444', borderRadius: 4, padding: '8px 12px', fontSize: 14, fontFamily: 'monospace' }} />
        {error && <div style={{ color: '#f66', fontSize: 12 }}>{error}</div>}
        <button type="submit" disabled={loading} style={{ background: '#4af', color: '#000', border: 'none', borderRadius: 4, padding: '8px 12px', fontSize: 14, cursor: 'pointer' }}>
          {loading ? '…' : 'Login'}
        </button>
      </form>
    </div>
  )
}

type AdminTab = 'realtime' | 'history' | 'analytics'

function AdminApp({ showLogout }: { showLogout: boolean }) {
  const initTab = (): AdminTab => {
    const p = window.location.pathname
    return p.startsWith('/admin/analytics') ? 'analytics' : p.startsWith('/admin/history') ? 'history' : 'realtime'
  }
  const [tab, setTab] = useState<AdminTab>(initTab)

  const navigate = (t: AdminTab) => {
    const url = t === 'realtime' ? '/admin' : t === 'history' ? '/admin/history' : '/admin/analytics'
    window.history.pushState({}, '', url)
    setTab(t)
  }

  const tabStyle = (active: boolean): React.CSSProperties => ({
    padding: '8px 16px', cursor: 'pointer',
    background: active ? '#222' : 'none',
    color: active ? '#4af' : '#888',
    border: 'none',
    borderBottom: active ? '2px solid #4af' : '2px solid transparent',
    fontSize: 13, fontFamily: 'sans-serif',
  })

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100dvh', background: '#0a0a0a', color: '#e0e0e0' }}>
      <div style={{ display: 'flex', background: '#111', borderBottom: '1px solid #222', flexShrink: 0, alignItems: 'center', padding: '0 8px' }}>
        <span style={{ color: '#4af', fontFamily: 'monospace', fontSize: 14, padding: '8px 12px', marginRight: 8 }}>Perch Admin</span>
        <button style={tabStyle(tab === 'realtime')} onClick={() => navigate('realtime')}>Live</button>
        <button style={tabStyle(tab === 'history')} onClick={() => navigate('history')}>History</button>
        <button style={tabStyle(tab === 'analytics')} onClick={() => navigate('analytics')}>Analytics</button>
        <a href="/chat" style={{ marginLeft: 8, color: '#888', fontSize: 12, fontFamily: 'sans-serif', textDecoration: 'none' }}>→ Chat</a>
        {showLogout && (
          <div style={{ marginLeft: 'auto' }}>
            <LogoutButton />
          </div>
        )}
      </div>
      <div style={{ flex: 1, overflowY: 'auto' }}>
        {tab === 'realtime' && <AdminRealtimePage />}
        {tab === 'history' && <AdminHistoryPage />}
        {tab === 'analytics' && <AdminAnalyticsPage />}
      </div>
    </div>
  )
}

// Loading spinner shown while auth status is fetching
function LoadingScreen() {
  return (
    <div style={{
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      height: '100dvh', background: '#1a1a1a', color: '#555', fontSize: 14,
      fontFamily: 'monospace',
    }}>
      Loading…
    </div>
  )
}

export default function App() {
  const path = window.location.pathname
  const [authStatus, setAuthStatus] = useState<AuthStatus | null>(null)
  const [accessDenied, setAccessDenied] = useState(false)

  useEffect(() => {
    // Parse ?error=access_denied from URL on mount
    const params = new URLSearchParams(window.location.search)
    if (params.get('error') === 'access_denied') {
      setAccessDenied(true)
    }

    // Fetch auth status
    fetch('/api/auth/status')
      .then(r => r.json())
      .then((s: AuthStatus) => setAuthStatus(s))
      .catch(() => {
        // Fallback: assume single-mode, no auth, authenticated
        setAuthStatus({ authenticated: true, username: '', role: '', mode: 'single', auth_method: 'none' })
      })
  }, [])

  // Admin login page is always served as-is
  if (path === '/admin/login') {
    return <AdminLoginPage />
  }

  // Show loading while auth status is in-flight
  if (authStatus === null) {
    return <LoadingScreen />
  }

  // Multi-user unauthenticated: show inline login screen
  if (authStatus.mode === 'multi' && !authStatus.authenticated) {
    return <GitLabLoginScreen error={accessDenied} />
  }

  // Route based on path
  if (path === '/terminal') {
    return <TerminalRoot status={authStatus} accessDenied={accessDenied} />
  }

  if (path.startsWith('/chat') || path === '/') {
    return <ChatApp authStatus={authStatus} />
  }

  if (path.startsWith('/admin')) {
    return <AdminApp showLogout={authStatus.authenticated && authStatus.auth_method !== 'none'} />
  }

  // Fallback: render ChatApp
  return <ChatApp authStatus={authStatus} />
}
