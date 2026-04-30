import { useEffect, useRef, useState } from 'react'

interface AdminSession {
  id: string
  username: string
  query: string
  status: string
  current_tool?: string
  started_at: number
}

function elapsed(startedAt: number): string {
  const ms = Date.now() - startedAt
  const s = Math.floor(ms / 1000)
  if (s < 60) return `${s}s`
  return `${Math.floor(s / 60)}m ${s % 60}s`
}

function useAdminWS() {
  const [sessions, setSessions] = useState<Record<string, AdminSession>>({})
  const wsRef = useRef<WebSocket | null>(null)

  useEffect(() => {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${location.host}/ws/management`)
    wsRef.current = ws

    ws.onmessage = (e) => {
      try {
        const ev = JSON.parse(e.data)
        if (ev.type === 'session_snapshot') {
          const map: Record<string, AdminSession> = {}
          for (const s of ev.sessions ?? []) map[s.id] = s
          setSessions(map)
        } else if (ev.type === 'session_added' && ev.session) {
          setSessions(prev => ({ ...prev, [ev.session.id]: ev.session }))
        } else if (ev.type === 'session_update' && ev.id) {
          setSessions(prev => {
            if (!prev[ev.id]) return prev
            return { ...prev, [ev.id]: { ...prev[ev.id], current_tool: ev.session?.current_tool ?? '' } }
          })
        } else if (ev.type === 'session_removed' && ev.id) {
          setSessions(prev => {
            const next = { ...prev }
            delete next[ev.id]
            return next
          })
        }
      } catch { /* ignore */ }
    }

    ws.onerror = () => ws.close()
    return () => ws.close()
  }, [])

  return sessions
}

export default function AdminRealtimePage() {
  const sessions = useAdminWS()
  const [tick, setTick] = useState(0)

  // Update elapsed time every second
  useEffect(() => {
    const id = setInterval(() => setTick(t => t + 1), 1000)
    return () => clearInterval(id)
  }, [])

  const list = Object.values(sessions)

  return (
    <div style={{ padding: 24, fontFamily: 'sans-serif', color: '#e0e0e0' }}>
      <h2 style={{ margin: '0 0 16px', fontSize: 18 }}>
        Live Sessions {list.length > 0 && <span style={{ color: '#4af', fontSize: 14 }}>({list.length})</span>}
      </h2>

      {list.length === 0 ? (
        <div style={{ color: '#555', marginTop: 40, textAlign: 'center' }}>No active sessions</div>
      ) : (
        <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
          <thead>
            <tr style={{ borderBottom: '1px solid #333', color: '#888' }}>
              <th style={{ textAlign: 'left', padding: '8px 12px' }}>User</th>
              <th style={{ textAlign: 'left', padding: '8px 12px' }}>Query</th>
              <th style={{ textAlign: 'left', padding: '8px 12px' }}>Elapsed</th>
              <th style={{ textAlign: 'left', padding: '8px 12px' }}>Current Tool</th>
              <th style={{ textAlign: 'left', padding: '8px 12px' }}>Status</th>
            </tr>
          </thead>
          <tbody>
            {list.map(sess => (
              <tr key={sess.id} style={{ borderBottom: '1px solid #222' }}>
                <td style={{ padding: '8px 12px', color: '#4af' }}>{sess.username}</td>
                <td style={{ padding: '8px 12px', maxWidth: 300, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {sess.query}
                </td>
                <td style={{ padding: '8px 12px', color: '#aaa' }}>{elapsed(sess.started_at)}</td>
                <td style={{ padding: '8px 12px', color: sess.current_tool ? '#fa0' : '#555' }}>
                  {sess.current_tool || '—'}
                </td>
                <td style={{ padding: '8px 12px' }}>
                  <span style={{
                    background: sess.status === 'running' ? '#1a4' : '#444',
                    color: '#fff', borderRadius: 4, padding: '2px 8px', fontSize: 11,
                  }}>
                    {sess.status}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <div style={{ display: 'none' }}>{tick}</div>
    </div>
  )
}
