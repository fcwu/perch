import { useEffect, useRef, useState, useCallback } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { Keyboard } from './Keyboard'
import ChatPage from './ChatPage'
import AdminRealtimePage from './AdminRealtimePage'
import AdminHistoryPage from './AdminHistoryPage'
import AdminAnalyticsPage from './AdminAnalyticsPage'

const URL_RE = /https?:\/\/[^\s\x00-\x1f\x7f]+/g

interface SessionView {
  channel_id: string
  session_uuid: string
}

type ActiveTab = null | string  // null = main PTY; string = discord channelID

function useDiscordSessions() {
  const [sessions, setSessions] = useState<SessionView[]>([])
  useEffect(() => {
    const poll = async () => {
      try {
        const r = await fetch('/sessions')
        if (r.ok) setSessions(await r.json())
      } catch { /* ignore network errors */ }
    }
    poll()
    const id = setInterval(poll, 5000)
    return () => clearInterval(id)
  }, [])
  return sessions
}

// userIDFromCookie reads the userID from the perch_session cookie (server sets it).
// The cookie is HttpOnly so JS can't read it directly; the server injects it via a meta tag or
// we parse it from the URL query param that the server can embed on the /chat page.
// For now, the ChatPage derives userID via /api/chat response.
// This sentinel lets ChatPage request userID lazily.
const CHAT_USER_PLACEHOLDER = 'me'

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

function TerminalApp() {
  const containerRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)
  const fitAddonRef = useRef<FitAddon | null>(null)
  const sessions = useDiscordSessions()
  const [activeTab, setActiveTab] = useState<ActiveTab>(null)

  // Initial terminal setup — runs once on mount
  useEffect(() => {
    const term = new Terminal({
      cursorBlink: true,
      fontFamily: 'monospace',
      fontSize: 14,
    })
    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)

    // Custom link provider that handles URLs wrapping across multiple terminal
    // rows. WebLinksAddon only matches within a single row; this handles two
    // cases: (1) isWrapped=true terminal wrapping, and (2) explicit \n splits
    // used by Claude Code for long OAuth URLs where each line has no whitespace.
    term.registerLinkProvider({
      provideLinks(bufferRow, callback) {
        const buffer = term.buffer.active
        // buffer.getLine() is 0-indexed; bufferRow is 1-indexed
        const getLine = (r: number) => buffer.getLine(r - 1)
        const rowText = (r: number) => (getLine(r)?.translateToString(false) ?? '').trimEnd()
        // A row with no whitespace looks like a URL fragment (percent-encoded, no spaces)
        const noSpace = (s: string) => s.length > 0 && !/\s/.test(s)

        // Walk back to find the first row of this group.
        // Handles isWrapped=true continuations AND explicit-newline URL splits:
        // if the current row has no spaces and doesn't start a new URL, and
        // the previous row also has no spaces, they're likely parts of the same URL.
        let startRow = bufferRow
        while (startRow > 1) {
          if (getLine(startRow)?.isWrapped) { startRow--; continue }
          const t = rowText(startRow)
          if (noSpace(t) && !t.startsWith('http') && noSpace(rowText(startRow - 1))) {
            startRow--; continue
          }
          break
        }

        // Collect all rows going forward (isWrapped continuations + no-space URL fragments)
        const rows: number[] = [startRow]
        for (let r = startRow; r <= buffer.length; r++) {
          const next = getLine(r + 1)
          if (!next) break
          if (next.isWrapped) {
            rows.push(r + 1)
          } else {
            // Extend for explicit-newline URL fragments: both current and next row
            // must have no whitespace, and next must not start a new URL.
            const nextT = rowText(r + 1)
            if (noSpace(rowText(r)) && noSpace(nextT) && !nextT.startsWith('http')) {
              rows.push(r + 1)
            } else {
              break
            }
          }
        }

        if (!rows.includes(bufferRow)) { callback(undefined); return }

        // Get each row's text trimmed of padding spaces
        const trimmed = rows.map(r => rowText(r))
        const combined = trimmed.join('')

        // Cumulative character lengths (for offset → coordinate mapping)
        const cumLen: number[] = [0]
        for (const s of trimmed) cumLen.push(cumLen[cumLen.length - 1] + s.length)

        // Map a character offset in `combined` back to buffer {x, y} coordinates
        const toCoord = (off: number) => {
          let ri = trimmed.length - 1
          for (let i = 0; i < trimmed.length; i++) {
            if (off < cumLen[i + 1]) { ri = i; break }
          }
          return { y: rows[ri], x: (off - cumLen[ri]) + 1 }
        }

        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const links: any[] = []
        URL_RE.lastIndex = 0
        let m: RegExpExecArray | null
        while ((m = URL_RE.exec(combined)) !== null) {
          const uri = m[0]
          const s = m.index
          const e = s + uri.length - 1
          links.push({
            text: uri,
            range: { start: toCoord(s), end: toCoord(e) },
            decorations: { pointerCursor: true, underline: true },
            activate: (_: MouseEvent, text: string) => {
              // Use anchor click instead of window.open to avoid popup blockers
              const a = document.createElement('a')
              a.href = text
              a.target = '_blank'
              a.rel = 'noopener noreferrer'
              a.click()
            },
          })
        }
        callback(links.length ? links : undefined)
      },
    })

    term.open(containerRef.current!)
    fitAddon.fit()
    termRef.current = term
    fitAddonRef.current = fitAddon

    return () => {
      term.dispose()
      termRef.current = null
      fitAddonRef.current = null
    }
  }, [])

  // Connect/reconnect WebSocket whenever active tab changes
  const connectWS = useCallback((tab: ActiveTab) => {
    const term = termRef.current
    const fitAddon = fitAddonRef.current
    if (!term || !fitAddon || !containerRef.current) return

    // Clear terminal buffer so the new session starts with a clean view.
    term.reset()

    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    const url = tab === null
      ? `${proto}://${location.host}/ws`
      : `${proto}://${location.host}/ws/session?id=${encodeURIComponent(tab)}`

    const ws = new WebSocket(url)
    ws.binaryType = 'arraybuffer'

    ws.onopen = () => {
      fitAddon.fit()
      const { cols, rows } = term
      ws.send(JSON.stringify({ type: 'resize', cols, rows }))
    }

    ws.onmessage = (e) => {
      term.write(new Uint8Array(e.data as ArrayBuffer))
    }

    const disposable = term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data))
      }
    })
    const disposeOnData = () => disposable.dispose()

    const sendResize = () => {
      fitAddon.fit()
      const { cols, rows } = term
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols, rows }))
      }
    }

    const observer = new ResizeObserver(sendResize)
    observer.observe(containerRef.current!)
    window.addEventListener('resize', sendResize)

    // When the native mobile keyboard appears the visual viewport shrinks.
    // Resize the terminal to fit the remaining space and scroll to bottom so
    // the cursor line stays visible.
    const handleViewportResize = () => {
      sendResize()
      term.scrollToBottom()
    }
    window.visualViewport?.addEventListener('resize', handleViewportResize)

    return () => {
      observer.disconnect()
      window.removeEventListener('resize', sendResize)
      window.visualViewport?.removeEventListener('resize', handleViewportResize)
      disposeOnData?.()
      ws.close()
    }
  }, [])

  useEffect(() => {
    return connectWS(activeTab)
  }, [activeTab, connectWS])

  const tabStyle = (active: boolean): React.CSSProperties => ({
    padding: '4px 12px',
    cursor: 'pointer',
    background: active ? '#333' : '#111',
    color: active ? '#fff' : '#888',
    border: 'none',
    borderBottom: active ? '2px solid #4af' : '2px solid transparent',
    fontSize: 12,
    fontFamily: 'monospace',
    flexShrink: 0,
  })

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100dvh', background: '#000' }}>
      {sessions.length > 0 && (
        <div style={{ display: 'flex', background: '#111', flexShrink: 0, overflowX: 'auto' }}>
          <button style={tabStyle(activeTab === null)} onClick={() => setActiveTab(null)}>
            Terminal
          </button>
          {sessions.map(sess => (
            <button
              key={sess.channel_id}
              style={tabStyle(activeTab === sess.channel_id)}
              onClick={() => setActiveTab(sess.channel_id)}
            >
              Discord {sess.channel_id}
            </button>
          ))}
        </div>
      )}
      <div ref={containerRef} style={{ flex: 1, overflow: 'hidden' }} />
      <Keyboard termRef={termRef} />
    </div>
  )
}

function TerminalRoot() {
  const [authChecked, setAuthChecked] = useState(false)
  const [authenticated, setAuthenticated] = useState(true)

  useEffect(() => {
    fetch('/api/auth').then(r => {
      setAuthenticated(r.ok)
      setAuthChecked(true)
    }).catch(() => {
      // /api/auth not registered (AUTH_MODE != password) → assume authenticated
      setAuthChecked(true)
    })
  }, [])

  if (!authChecked) return null
  if (!authenticated) {
    return <LoginOverlay onLogin={() => setAuthenticated(true)} />
  }
  return <TerminalApp />
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

function AdminApp() {
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
      </div>
      <div style={{ flex: 1, overflowY: 'auto' }}>
        {tab === 'realtime' && <AdminRealtimePage />}
        {tab === 'history' && <AdminHistoryPage />}
        {tab === 'analytics' && <AdminAnalyticsPage />}
      </div>
    </div>
  )
}

export default function App() {
  const path = window.location.pathname
  if (path.startsWith('/chat')) {
    return <ChatPage userID={CHAT_USER_PLACEHOLDER} />
  }
  if (path === '/admin/login') {
    return <AdminLoginPage />
  }
  if (path.startsWith('/admin')) {
    return <AdminApp />
  }
  return <TerminalRoot />
}
