import { useEffect, useState, useCallback } from 'react'

interface Schedule {
  id: string
  hour?: number
  minute?: number
  repeat: boolean
  one_shot_at: number
  prompt: string
  enabled: boolean
  created_at: number
  last_fired_at?: number
}

interface SchedulePanelProps {
  conversationId: string
  onClose: () => void
}

export default function SchedulePanel({ conversationId, onClose }: SchedulePanelProps) {
  const [schedules, setSchedules] = useState<Schedule[]>([])
  const [mode, setMode] = useState<'daily' | 'oneshot'>('daily')
  const [hour, setHour] = useState(9)
  const [minute, setMinute] = useState(0)
  const [repeat, setRepeat] = useState(true)
  const [oneShotAt, setOneShotAt] = useState('')
  const [prompt, setPrompt] = useState('')
  const [error, setError] = useState('')
  const FONT = "-apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif"

  const load = useCallback(async () => {
    try {
      const r = await fetch(`/api/conversations/${conversationId}/schedules`)
      if (!r.ok) return
      const data = await r.json()
      setSchedules(data.schedules ?? [])
    } catch { /* ignore */ }
  }, [conversationId])

  useEffect(() => { load() }, [load])

  const handleCreate = async () => {
    setError('')
    if (!prompt.trim()) {
      setError('Prompt is required')
      return
    }
    const body: Record<string, unknown> = { prompt }
    if (mode === 'daily') {
      body.hour = hour
      body.minute = minute
      body.repeat = repeat
    } else {
      const ms = Date.parse(oneShotAt)
      if (Number.isNaN(ms) || ms <= Date.now()) {
        setError('One-shot time must be in the future')
        return
      }
      body.one_shot_at = ms
    }
    const r = await fetch(`/api/conversations/${conversationId}/schedules`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!r.ok) {
      setError('Failed: ' + (await r.text()))
      return
    }
    setPrompt('')
    setOneShotAt('')
    await load()
  }

  const handleDelete = async (id: string) => {
    await fetch(`/api/conversations/${conversationId}/schedules/${id}`, { method: 'DELETE' })
    setSchedules(prev => prev.filter(s => s.id !== id))
  }

  return (
    <div style={{
      position: 'fixed', top: 0, right: 0, width: 360, height: '100dvh',
      background: '#1e1e1e', borderLeft: '1px solid #333',
      boxShadow: '-4px 0 16px rgba(0,0,0,0.4)',
      display: 'flex', flexDirection: 'column',
      fontFamily: FONT, color: '#e0e0e0',
      zIndex: 50,
    }}>
      <div style={{ padding: '14px 16px', borderBottom: '1px solid #2e2e2e', display: 'flex', alignItems: 'center' }}>
        <div style={{ flex: 1, fontSize: 14, fontWeight: 600 }}>Schedules</div>
        <button onClick={onClose} style={{
          background: 'none', border: 'none', color: '#888',
          cursor: 'pointer', fontSize: 18, padding: '2px 6px',
        }} aria-label="close">✕</button>
      </div>
      <div style={{ flex: 1, overflowY: 'auto', padding: '12px 16px' }}>
        {schedules.length === 0 && (
          <div style={{ color: '#666', fontSize: 12 }}>No schedules yet</div>
        )}
        {schedules.map(s => (
          <div key={s.id} style={{
            padding: '8px 10px', marginBottom: 8, background: '#262626',
            border: '1px solid #333', borderRadius: 8, display: 'flex',
            flexDirection: 'column', gap: 4,
          }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
              <span style={{ fontSize: 12, color: '#aaa' }}>
                {s.one_shot_at > 0
                  ? `Once @ ${new Date(s.one_shot_at).toLocaleString()}`
                  : `Daily ${String(s.hour ?? 0).padStart(2, '0')}:${String(s.minute ?? 0).padStart(2, '0')}${s.repeat ? '' : ' (once)'}`}
              </span>
              <button
                onClick={() => handleDelete(s.id)}
                style={{
                  marginLeft: 'auto',
                  background: 'none', border: 'none', color: '#888',
                  cursor: 'pointer', fontSize: 13, padding: '2px 4px',
                }}
                aria-label={`delete ${s.id}`}
              >🗑</button>
            </div>
            <div style={{ fontSize: 13, color: '#ddd', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
              {s.prompt}
            </div>
          </div>
        ))}
      </div>
      <div style={{ padding: '12px 16px', borderTop: '1px solid #2e2e2e' }}>
        <div style={{ display: 'flex', gap: 8, marginBottom: 8 }}>
          <button
            onClick={() => setMode('daily')}
            style={{
              flex: 1, background: mode === 'daily' ? '#2e2e2e' : 'transparent',
              border: '1px solid #333', color: '#ddd', borderRadius: 6,
              padding: '6px 10px', cursor: 'pointer', fontSize: 12,
            }}
          >Daily</button>
          <button
            onClick={() => setMode('oneshot')}
            style={{
              flex: 1, background: mode === 'oneshot' ? '#2e2e2e' : 'transparent',
              border: '1px solid #333', color: '#ddd', borderRadius: 6,
              padding: '6px 10px', cursor: 'pointer', fontSize: 12,
            }}
          >One-shot</button>
        </div>
        {mode === 'daily' ? (
          <div style={{ display: 'flex', gap: 6, marginBottom: 8, alignItems: 'center' }}>
            <input
              type="number" min={0} max={23} value={hour}
              onChange={e => setHour(Number(e.target.value))}
              style={{ width: 50, background: '#2a2a2a', border: '1px solid #333', color: '#ddd', padding: '4px 6px', borderRadius: 4 }}
              aria-label="hour"
            />
            :
            <input
              type="number" min={0} max={59} value={minute}
              onChange={e => setMinute(Number(e.target.value))}
              style={{ width: 50, background: '#2a2a2a', border: '1px solid #333', color: '#ddd', padding: '4px 6px', borderRadius: 4 }}
              aria-label="minute"
            />
            <label style={{ marginLeft: 8, fontSize: 12, color: '#aaa', display: 'flex', alignItems: 'center', gap: 4 }}>
              <input type="checkbox" checked={repeat} onChange={e => setRepeat(e.target.checked)} /> Repeat
            </label>
          </div>
        ) : (
          <input
            type="datetime-local"
            value={oneShotAt}
            onChange={e => setOneShotAt(e.target.value)}
            style={{ width: '100%', background: '#2a2a2a', border: '1px solid #333', color: '#ddd', padding: '6px 8px', borderRadius: 4, marginBottom: 8 }}
            aria-label="one-shot datetime"
          />
        )}
        <textarea
          value={prompt}
          onChange={e => setPrompt(e.target.value)}
          placeholder="Prompt to send"
          rows={2}
          style={{
            width: '100%', background: '#2a2a2a', border: '1px solid #333',
            color: '#ddd', padding: '6px 8px', borderRadius: 4, marginBottom: 8,
            fontFamily: FONT, fontSize: 13, resize: 'vertical',
          }}
          aria-label="schedule prompt"
        />
        {error && (
          <div style={{ color: '#ff6b6b', fontSize: 12, marginBottom: 6 }}>{error}</div>
        )}
        <button
          onClick={handleCreate}
          style={{
            width: '100%', background: '#4a9eff', border: 'none',
            color: '#fff', borderRadius: 6, padding: '8px 10px',
            cursor: 'pointer', fontSize: 13,
          }}
        >Create</button>
      </div>
    </div>
  )
}
