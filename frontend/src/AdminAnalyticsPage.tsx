import { useEffect, useState } from 'react'

interface UserStat {
  username: string
  query_count: number
  avg_duration_ms: number
}

interface ToolStat {
  tool: string
  count: number
}

interface UsageStats {
  users: UserStat[]
  top_tools: ToolStat[]
  total_queries: number
  total_duration_ms: number
}

type Range = 'today' | 'week' | 'month'

function rangeToParams(r: Range): { from: number; to: number } {
  const now = Date.now()
  const day = 24 * 60 * 60 * 1000
  switch (r) {
    case 'today':
      return { from: new Date().setHours(0, 0, 0, 0), to: now }
    case 'week':
      return { from: now - 7 * day, to: now }
    case 'month':
      return { from: now - 30 * day, to: now }
  }
}

export default function AdminAnalyticsPage() {
  const [range, setRange] = useState<Range>('week')
  const [stats, setStats] = useState<UsageStats | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    const { from, to } = rangeToParams(range)
    setLoading(true)
    fetch(`/admin/analytics?from=${from}&to=${to}`)
      .then(r => r.ok ? r.json() : null)
      .then(d => setStats(d))
      .finally(() => setLoading(false))
  }, [range])

  const btnStyle = (active: boolean): React.CSSProperties => ({
    background: active ? '#4af' : '#222',
    color: active ? '#000' : '#aaa',
    border: '1px solid #444',
    borderRadius: 4,
    padding: '4px 12px',
    cursor: 'pointer',
    fontSize: 13,
  })

  return (
    <div style={{ padding: 24, fontFamily: 'sans-serif', color: '#e0e0e0' }}>
      <h2 style={{ margin: '0 0 16px', fontSize: 18 }}>Analytics</h2>

      <div style={{ display: 'flex', gap: 8, marginBottom: 24 }}>
        {(['today', 'week', 'month'] as Range[]).map(r => (
          <button key={r} style={btnStyle(range === r)} onClick={() => setRange(r)}>
            {r === 'today' ? 'Today' : r === 'week' ? 'This Week' : 'This Month'}
          </button>
        ))}
      </div>

      {loading ? (
        <div style={{ color: '#555', textAlign: 'center', marginTop: 40 }}>Loading…</div>
      ) : !stats ? (
        <div style={{ color: '#555', textAlign: 'center', marginTop: 40 }}>No data</div>
      ) : (
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 24 }}>
          {/* Summary */}
          <div style={{ gridColumn: '1 / -1', display: 'flex', gap: 24 }}>
            {[
              { label: 'Total Queries', value: stats.total_queries },
              { label: 'Avg Duration', value: stats.total_queries > 0 ? `${(stats.total_duration_ms / stats.total_queries / 1000).toFixed(1)}s` : '—' },
              { label: 'Active Users', value: stats.users.length },
            ].map(card => (
              <div key={card.label} style={{ flex: 1, background: '#111', borderRadius: 8, padding: '16px 20px' }}>
                <div style={{ color: '#888', fontSize: 12, marginBottom: 4 }}>{card.label}</div>
                <div style={{ fontSize: 24, fontWeight: 600 }}>{card.value}</div>
              </div>
            ))}
          </div>

          {/* User stats */}
          <div>
            <h3 style={{ fontSize: 14, color: '#888', margin: '0 0 12px' }}>Users</h3>
            {stats.users.length === 0 ? (
              <div style={{ color: '#555', fontSize: 13 }}>No data</div>
            ) : (
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
                <thead>
                  <tr style={{ color: '#666', borderBottom: '1px solid #222' }}>
                    <th style={{ textAlign: 'left', padding: '6px 8px' }}>User</th>
                    <th style={{ textAlign: 'right', padding: '6px 8px' }}>Queries</th>
                    <th style={{ textAlign: 'right', padding: '6px 8px' }}>Avg Duration</th>
                  </tr>
                </thead>
                <tbody>
                  {stats.users.map(u => (
                    <tr key={u.username} style={{ borderBottom: '1px solid #1a1a1a' }}>
                      <td style={{ padding: '6px 8px', color: '#4af' }}>{u.username}</td>
                      <td style={{ padding: '6px 8px', textAlign: 'right' }}>{u.query_count}</td>
                      <td style={{ padding: '6px 8px', textAlign: 'right', color: '#888' }}>
                        {(u.avg_duration_ms / 1000).toFixed(1)}s
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>

          {/* Top tools */}
          <div>
            <h3 style={{ fontSize: 14, color: '#888', margin: '0 0 12px' }}>Top Tools</h3>
            {stats.top_tools.length === 0 ? (
              <div style={{ color: '#555', fontSize: 13 }}>No data</div>
            ) : (
              <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
                <thead>
                  <tr style={{ color: '#666', borderBottom: '1px solid #222' }}>
                    <th style={{ textAlign: 'left', padding: '6px 8px' }}>Tool</th>
                    <th style={{ textAlign: 'right', padding: '6px 8px' }}>Uses</th>
                  </tr>
                </thead>
                <tbody>
                  {stats.top_tools.map(t => (
                    <tr key={t.tool} style={{ borderBottom: '1px solid #1a1a1a' }}>
                      <td style={{ padding: '6px 8px', color: '#fa0' }}>{t.tool}</td>
                      <td style={{ padding: '6px 8px', textAlign: 'right' }}>{t.count}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
