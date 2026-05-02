import { useCallback, useEffect, useState } from 'react'

interface Conversation {
  id: string
  user_id: string
  title: string
  created_at: number
  updated_at: number
  pinned: boolean
  runtime?: string
  model?: string
}

interface ConvMessage {
  id: string
  user_id: string
  query: string
  response?: string
  status: string
  source: string
  started_at: number
  ended_at?: number
}

function formatDate(ms: number): string {
  return new Date(ms).toLocaleString()
}

export default function AdminConversationsPage() {
  const [user, setUser] = useState('')
  const [keyword, setKeyword] = useState('')
  const [convs, setConvs] = useState<Conversation[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [selected, setSelected] = useState<Conversation | null>(null)
  const [messages, setMessages] = useState<ConvMessage[]>([])
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    const params = new URLSearchParams({ page: String(page), limit: '20' })
    if (user) params.set('user', user)
    if (keyword) params.set('q', keyword)
    try {
      const r = await fetch(`/api/management/conversations?${params}`)
      if (r.ok) {
        const data = await r.json()
        setConvs(data.conversations ?? [])
        setTotal(data.total ?? 0)
      }
    } finally {
      setLoading(false)
    }
  }, [user, keyword, page])

  useEffect(() => { load() }, [load])

  const openDetail = async (conv: Conversation) => {
    setSelected(conv)
    const r = await fetch(`/api/management/conversations/${conv.id}/messages?limit=200`)
    if (r.ok) {
      const data = await r.json()
      setMessages(data.messages ?? [])
    } else {
      setMessages([])
    }
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
            user: {selected.user_id} · runtime: {selected.runtime ?? '—'} · model: {selected.model ?? '—'} · updated: {formatDate(selected.updated_at)}
          </div>
          <div style={{ fontSize: 16 }}>{selected.title}</div>
        </div>
        <h3 style={{ fontSize: 14, color: '#888', margin: '16px 0 8px' }}>Messages</h3>
        {messages.length === 0 && (
          <div style={{ color: '#555' }}>No messages</div>
        )}
        {messages.map(m => (
          <div key={m.id} style={{ background: '#111', borderRadius: 6, padding: 10, marginBottom: 8, fontSize: 12 }}>
            <div style={{ color: '#888', fontSize: 11, marginBottom: 4, display: 'flex', gap: 8, alignItems: 'center' }}>
              <span>{formatDate(m.started_at)}</span>
              <span>·</span>
              <span style={{ color: m.status === 'done' ? '#4af' : m.status === 'running' ? '#aa4' : '#a44' }}>{m.status}</span>
              {m.source === 'schedule' && (
                <span title="Triggered by schedule" style={{ color: '#ffd166' }}>⏰</span>
              )}
            </div>
            <pre style={{ margin: 0, whiteSpace: 'pre-wrap', wordBreak: 'break-word', color: '#ddd' }}>{m.query}</pre>
            {m.response && (
              <pre style={{ margin: '6px 0 0', whiteSpace: 'pre-wrap', wordBreak: 'break-word', color: '#bbb', borderTop: '1px solid #222', paddingTop: 6 }}>{m.response}</pre>
            )}
          </div>
        ))}
      </div>
    )
  }

  return (
    <div style={{ padding: 24, fontFamily: 'sans-serif', color: '#e0e0e0' }}>
      <h2 style={{ margin: '0 0 16px', fontSize: 18 }}>Conversations</h2>
      <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
        <input
          placeholder="Filter by user"
          value={user}
          onChange={e => { setUser(e.target.value); setPage(1) }}
          style={{ background: '#222', border: '1px solid #444', color: '#e0e0e0', borderRadius: 4, padding: '6px 10px', fontSize: 13 }}
        />
        <input
          placeholder="Search title…"
          value={keyword}
          onChange={e => { setKeyword(e.target.value); setPage(1) }}
          style={{ flex: 1, background: '#222', border: '1px solid #444', color: '#e0e0e0', borderRadius: 4, padding: '6px 10px', fontSize: 13 }}
        />
      </div>
      {loading ? (
        <div style={{ color: '#555', textAlign: 'center', marginTop: 40 }}>Loading…</div>
      ) : convs.length === 0 ? (
        <div style={{ color: '#555', textAlign: 'center', marginTop: 40 }}>No conversations</div>
      ) : (
        <>
          <div style={{ color: '#555', fontSize: 12, marginBottom: 8 }}>{total} conversations total</div>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
            <thead>
              <tr style={{ borderBottom: '1px solid #333', color: '#888' }}>
                <th style={{ textAlign: 'left', padding: '6px 10px' }}>User</th>
                <th style={{ textAlign: 'left', padding: '6px 10px' }}>Title</th>
                <th style={{ textAlign: 'left', padding: '6px 10px' }}>Runtime</th>
                <th style={{ textAlign: 'left', padding: '6px 10px' }}>Model</th>
                <th style={{ textAlign: 'left', padding: '6px 10px' }}>Updated</th>
              </tr>
            </thead>
            <tbody>
              {convs.map(c => (
                <tr
                  key={c.id}
                  onClick={() => openDetail(c)}
                  style={{ borderBottom: '1px solid #1a1a1a', cursor: 'pointer' }}
                  onMouseEnter={e => (e.currentTarget.style.background = '#1a1a1a')}
                  onMouseLeave={e => (e.currentTarget.style.background = '')}
                >
                  <td style={{ padding: '8px 10px', color: '#4af' }}>{c.user_id}</td>
                  <td style={{ padding: '8px 10px', maxWidth: 360, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {c.pinned && <span title="Pinned" style={{ color: '#ffd166', marginRight: 6 }}>📌</span>}
                    {c.title}
                  </td>
                  <td style={{ padding: '8px 10px', color: '#aaa' }}>{c.runtime ?? '—'}</td>
                  <td style={{ padding: '8px 10px', color: '#aaa' }}>{c.model ?? '—'}</td>
                  <td style={{ padding: '8px 10px', color: '#888', fontSize: 12 }}>{formatDate(c.updated_at)}</td>
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
