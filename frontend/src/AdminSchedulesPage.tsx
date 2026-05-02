import { useCallback, useEffect, useState } from 'react'

interface Schedule {
  id: string
  user_id: string
  conversation_id: string
  hour?: number
  minute?: number
  repeat: boolean
  one_shot_at: number
  prompt: string
  enabled: boolean
  created_at: number
  last_fired_at?: number
}

function formatDate(ms?: number): string {
  if (!ms) return '—'
  return new Date(ms).toLocaleString()
}

function formatTrigger(s: Schedule): string {
  if (s.one_shot_at > 0) return `Once @ ${formatDate(s.one_shot_at)}`
  return `Daily ${String(s.hour ?? 0).padStart(2, '0')}:${String(s.minute ?? 0).padStart(2, '0')}${s.repeat ? '' : ' (once)'}`
}

export default function AdminSchedulesPage() {
  const [user, setUser] = useState('')
  const [conv, setConv] = useState('')
  const [rows, setRows] = useState<Schedule[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    const params = new URLSearchParams({ page: String(page), limit: '20' })
    if (user) params.set('user', user)
    if (conv) params.set('conv', conv)
    try {
      const r = await fetch(`/api/management/schedules?${params}`)
      if (r.ok) {
        const data = await r.json()
        setRows(data.schedules ?? [])
        setTotal(data.total ?? 0)
      }
    } finally {
      setLoading(false)
    }
  }, [user, conv, page])

  useEffect(() => { load() }, [load])

  return (
    <div style={{ padding: 24, fontFamily: 'sans-serif', color: '#e0e0e0' }}>
      <h2 style={{ margin: '0 0 16px', fontSize: 18 }}>Schedules</h2>
      <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
        <input
          placeholder="Filter by user"
          value={user}
          onChange={e => { setUser(e.target.value); setPage(1) }}
          style={{ background: '#222', border: '1px solid #444', color: '#e0e0e0', borderRadius: 4, padding: '6px 10px', fontSize: 13 }}
        />
        <input
          placeholder="Filter by conversation_id"
          value={conv}
          onChange={e => { setConv(e.target.value); setPage(1) }}
          style={{ flex: 1, background: '#222', border: '1px solid #444', color: '#e0e0e0', borderRadius: 4, padding: '6px 10px', fontSize: 13 }}
        />
      </div>
      {loading ? (
        <div style={{ color: '#555', textAlign: 'center', marginTop: 40 }}>Loading…</div>
      ) : rows.length === 0 ? (
        <div style={{ color: '#555', textAlign: 'center', marginTop: 40 }}>No schedules</div>
      ) : (
        <>
          <div style={{ color: '#555', fontSize: 12, marginBottom: 8 }}>{total} schedules total</div>
          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
            <thead>
              <tr style={{ borderBottom: '1px solid #333', color: '#888' }}>
                <th style={{ textAlign: 'left', padding: '6px 10px' }}>User</th>
                <th style={{ textAlign: 'left', padding: '6px 10px' }}>Conversation</th>
                <th style={{ textAlign: 'left', padding: '6px 10px' }}>Trigger</th>
                <th style={{ textAlign: 'left', padding: '6px 10px' }}>Prompt</th>
                <th style={{ textAlign: 'left', padding: '6px 10px' }}>Enabled</th>
                <th style={{ textAlign: 'left', padding: '6px 10px' }}>Last Fired</th>
              </tr>
            </thead>
            <tbody>
              {rows.map(s => (
                <tr key={s.id} style={{ borderBottom: '1px solid #1a1a1a' }}>
                  <td style={{ padding: '8px 10px', color: '#4af' }}>{s.user_id}</td>
                  <td style={{ padding: '8px 10px', color: '#aaa', fontFamily: 'monospace' }}>{s.conversation_id.slice(0, 8)}…</td>
                  <td style={{ padding: '8px 10px', color: '#ddd' }}>{formatTrigger(s)}</td>
                  <td style={{ padding: '8px 10px', maxWidth: 320, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{s.prompt}</td>
                  <td style={{ padding: '8px 10px', color: s.enabled ? '#4f4' : '#555' }}>{s.enabled ? 'on' : 'off'}</td>
                  <td style={{ padding: '8px 10px', color: '#888', fontSize: 12 }}>{formatDate(s.last_fired_at)}</td>
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
