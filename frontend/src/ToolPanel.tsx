import { useEffect, useState } from 'react'

interface ToolEntry {
  id: number
  tool: string
  input: string
  startTime: number
  elapsedMs?: number
  done: boolean
}

interface ToolPanelProps {
  entries: ToolEntry[]
  loading: boolean
}

const SPINNERS = ['⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏']

function ActivityBar({ entries, loading }: ToolPanelProps) {
  const [tick, setTick] = useState(0)

  const hasRunning = entries.some(e => !e.done)
  useEffect(() => {
    if (!hasRunning && !loading) return
    const t = setInterval(() => setTick(n => n + 1), 100)
    return () => clearInterval(t)
  }, [hasRunning, loading])

  void tick // used to force re-render

  const now = Date.now()
  const spin = SPINNERS[Math.floor(now / 100) % SPINNERS.length]

  const running = entries.filter(e => !e.done)
  const done = entries.filter(e => e.done)

  if (!loading && running.length === 0) return null

  return (
    <div style={{ borderTop: '1px solid #2a2a2a', background: '#0d0d0d', flexShrink: 0, padding: '4px 0' }}>
      {/* Running tools */}
      {running.map(e => (
        <div key={e.id} style={{
          padding: '2px 14px', fontSize: 12, fontFamily: 'monospace',
          color: '#ccc', display: 'flex', gap: 8, alignItems: 'center',
        }}>
          <span style={{ color: '#fa0' }}>{spin}</span>
          <span style={{ color: '#8cf' }}>{e.tool}</span>
          {e.input && (
            <span style={{ color: '#555', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', flex: 1 }}>
              {e.input.length > 70 ? e.input.slice(0, 70) + '…' : e.input}
            </span>
          )}
          <span style={{ color: '#666', flexShrink: 0 }}>
            {((now - e.startTime) / 1000).toFixed(1)}s
          </span>
        </div>
      ))}

      {/* Recent completed tools (last 5) */}
      {done.slice(-5).map(e => (
        <div key={e.id} style={{
          padding: '1px 14px', fontSize: 11, fontFamily: 'monospace',
          color: '#444', display: 'flex', gap: 8, alignItems: 'center',
        }}>
          <span style={{ color: '#2a2' }}>✓</span>
          <span>{e.tool}</span>
          {e.input && (
            <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', flex: 1 }}>
              {e.input.length > 70 ? e.input.slice(0, 70) + '…' : e.input}
            </span>
          )}
          {e.elapsedMs !== undefined && (
            <span style={{ flexShrink: 0 }}>{e.elapsedMs}ms</span>
          )}
        </div>
      ))}

      {/* Idle spinner when loading but no tool running */}
      {loading && running.length === 0 && (
        <div style={{ padding: '2px 14px', fontSize: 12, fontFamily: 'monospace', color: '#555' }}>
          <span style={{ color: '#fa0' }}>{spin}</span>
          <span style={{ marginLeft: 8 }}>thinking…</span>
        </div>
      )}
    </div>
  )
}

export function ToolPanel(props: ToolPanelProps) {
  return <ActivityBar {...props} />
}

export type { ToolEntry }
