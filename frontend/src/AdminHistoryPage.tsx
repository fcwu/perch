import { useCallback, useEffect, useState } from 'react'

interface SessionRow {
  ID: string
  Username: string
  Query: string
  Status: string
  StartedAt: number
  EndedAt?: number
  DurationMs?: number
}

interface ToolEvent {
  ToolName: string
  InputJSON?: string
  OutputJSON?: string
  StartedAt: number
  EndedAt?: number
}

interface SessionDetail extends SessionRow {
  Response?: string
  ToolEvents: ToolEvent[]
}

function formatDate(ms: number): string {
  return new Date(ms).toLocaleString()
}

function formatDuration(ms?: number): string {
  if (!ms) return '—'
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

export default function AdminHistoryPage() {
  const [user, setUser] = useState('')
  const [keyword, setKeyword] = useState('')
  const [sessions, setSessions] = useState<SessionRow[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<SessionDetail | null>(null)
  const [loading, setLoading] = useState(false)

  const fetchSessions = useCallback(async (u: string, q: string, p: number) => {
    setLoading(true)
    const params = new URLSearchParams({ page: String(p), limit: '20' })
    if (u) params.set('user', u)
    if (q) params.set('q', q)
    try {
      const r = await fetch(`/api/admin/history?${params}`)
      if (r.ok) {
        const data = await r.json()
        setSessions(data.sessions ?? [])
        setTotal(data.total ?? 0)
      }
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchSessions(user, keyword, page)
  }, [user, keyword, page, fetchSessions])

  const openDetail = async (id: string) => {
    const r = await fetch(`/api/admin/history/${id}`)
    if (r.ok) setSelected(await r.json())
  }

  if (selected) {
    return (
      <div style={{ padding: 24, fontFamily: 'sans-serif', color: '#e0e0e0' }}>
        <button onClick={() => setSelected(null)} style={{
          background: 'none', border: '1px solid #444', color: '#aaa',
          borderRadius: 4, padding: '4px 12px', cursor: 'pointer', marginBottom: 16,
        }}>← Back</button>

        <div style={{ marginBottom: 16 }}>
          <div style={{ color: '#888', fontSize: 12, marginBottom: 4 }}>
            {selected.Username} · {formatDate(selected.StartedAt)} · {formatDuration(selected.DurationMs)}
          </div>
          <div style={{ fontSize: 16, marginBottom: 12 }}>{selected.Query}</div>
          {selected.Response && (
            <pre style={{
              background: '#111', padding: 12, borderRadius: 6, fontSize: 12,
              whiteSpace: 'pre-wrap', wordBreak: 'break-word', maxHeight: 300, overflowY: 'auto',
            }}>
              {selected.Response}
            </pre>
          )}
        </div>

        {selected.ToolEvents?.length > 0 && (
          <>
            <h3 style={{ fontSize: 14, color: '#888', margin: '16px 0 8px' }}>Tool Calls</h3>
            {selected.ToolEvents.map((te, i) => (
              <div key={i} style={{ background: '#111', borderRadius: 6, padding: 10, marginBottom: 8, fontSize: 12 }}>
                <div style={{ color: '#fa0', marginBottom: 4 }}>{te.ToolName}</div>
                {te.InputJSON && <pre style={{ margin: 0, color: '#aaa', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>{te.InputJSON}</pre>}
              </div>
            ))}
          </>
        )}
      </div>
    )
  }

  return (
    <div style={{ padding: 24, fontFamily: 'sans-serif', color: '#e0e0e0' }}>
      <h2 style={{ margin: '0 0 16px', fontSize: 18 }}>Query History</h2>

      <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
        <input
          placeholder="Filter by user"
          value={user}
          onChange={e => { setUser(e.target.value); setPage(1) }}
          style={{ background: '#222', border: '1px solid #444', color: '#e0e0e0', borderRadius: 4, padding: '6px 10px', fontSize: 13 }}
        />
        <input
          placeholder="Search query…"
          value={keyword}
          onChange={e => { setKeyword(e.target.value); setPage(1) }}
          style={{ flex: 1, background: '#222', border: '1px solid #444', color: '#e0e0e0', borderRadius: 4, padding: '6px 10px', fontSize: 13 }}
        />
      </div>

      {loading ? (
        <div style={{ color: '#555', textAlign: 'center', marginTop: 40 }}>Loading…</div>
      ) : sessions.length === 0 ? (
        <div style={{ color: '#555', textAlign: 'center', marginTop: 40 }}>No sessions found</div>
      ) : (
        <>
          <div style={{ color: '#555', fontSize: 12, marginBottom: 8 }}>{total} sessions total</div>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
            <thead>
              <tr style={{ borderBottom: '1px solid #333', color: '#888' }}>
                <th style={{ textAlign: 'left', padding: '6px 10px' }}>User</th>
                <th style={{ textAlign: 'left', padding: '6px 10px' }}>Query</th>
                <th style={{ textAlign: 'left', padding: '6px 10px' }}>Time</th>
                <th style={{ textAlign: 'left', padding: '6px 10px' }}>Duration</th>
                <th style={{ textAlign: 'left', padding: '6px 10px' }}>Status</th>
              </tr>
            </thead>
            <tbody>
              {sessions.map(s => (
                <tr
                  key={s.ID}
                  onClick={() => openDetail(s.ID)}
                  style={{ borderBottom: '1px solid #1a1a1a', cursor: 'pointer' }}
                  onMouseEnter={e => (e.currentTarget.style.background = '#1a1a1a')}
                  onMouseLeave={e => (e.currentTarget.style.background = '')}
                >
                  <td style={{ padding: '8px 10px', color: '#4af' }}>{s.Username}</td>
                  <td style={{ padding: '8px 10px', maxWidth: 300, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{s.Query}</td>
                  <td style={{ padding: '8px 10px', color: '#888', fontSize: 12 }}>{formatDate(s.StartedAt)}</td>
                  <td style={{ padding: '8px 10px', color: '#888' }}>{formatDuration(s.DurationMs)}</td>
                  <td style={{ padding: '8px 10px' }}>
                    <span style={{
                      background: s.Status === 'done' ? '#1a4' : s.Status === 'running' ? '#44a' : '#a44',
                      color: '#fff', borderRadius: 4, padding: '2px 6px', fontSize: 11,
                    }}>
                      {s.Status}
                    </span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          <div style={{ display: 'flex', gap: 8, marginTop: 16, justifyContent: 'center' }}>
            <button onClick={() => setPage(p => Math.max(1, p - 1))} disabled={page === 1}
              style={{ background: '#222', border: '1px solid #444', color: '#aaa', borderRadius: 4, padding: '4px 12px', cursor: page === 1 ? 'default' : 'pointer' }}>
              Prev
            </button>
            <span style={{ padding: '4px 8px', color: '#888' }}>Page {page}</span>
            <button onClick={() => setPage(p => p + 1)} disabled={page * 20 >= total}
              style={{ background: '#222', border: '1px solid #444', color: '#aaa', borderRadius: 4, padding: '4px 12px', cursor: page * 20 >= total ? 'default' : 'pointer' }}>
              Next
            </button>
          </div>
        </>
      )}
    </div>
  )
}
