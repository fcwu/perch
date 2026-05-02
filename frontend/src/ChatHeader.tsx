import { useEffect, useState, useCallback } from 'react'

interface RuntimeView {
  id: string
  name: string
  models: string[]
  default_model: string
  supports_mcp: boolean
}

interface ChatHeaderProps {
  conversationId?: string
  onScheduleClick: () => void
}

export default function ChatHeader({ conversationId, onScheduleClick }: ChatHeaderProps) {
  const [runtimes, setRuntimes] = useState<RuntimeView[]>([])
  const [activeRuntime, setActiveRuntime] = useState('')
  const [activeModel, setActiveModel] = useState('')
  const FONT = "-apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif"

  const loadRuntimes = useCallback(async () => {
    try {
      const r = await fetch('/api/runtimes')
      if (!r.ok) return
      const data = await r.json()
      setRuntimes(data.runtimes ?? [])
    } catch { /* ignore */ }
  }, [])

  const loadConv = useCallback(async () => {
    if (!conversationId) {
      setActiveRuntime('')
      setActiveModel('')
      return
    }
    try {
      // The conversation list endpoint includes runtime/model on each row;
      // pull from there since we don't have a dedicated GET handler for the
      // user-scoped row.
      const r = await fetch(`/api/conversations?limit=100`)
      if (!r.ok) return
      const data = await r.json()
      const all = [...(data.pinned ?? []), ...(data.recent ?? [])]
      const conv = all.find((c: { id: string }) => c.id === conversationId)
      if (conv) {
        setActiveRuntime(conv.runtime ?? '')
        setActiveModel(conv.model ?? '')
      }
    } catch { /* ignore */ }
  }, [conversationId])

  useEffect(() => {
    loadRuntimes()
  }, [loadRuntimes])

  useEffect(() => {
    loadConv()
  }, [loadConv])

  const onChange = async (runtimeId: string, modelId: string) => {
    if (!conversationId) {
      // No active conversation yet — store locally so the next POST /api/chat
      // can include it as the new-conversation override. Surfaced via window
      // for ChatPage to read.
      ;(window as unknown as { perchPreferredRuntime?: string }).perchPreferredRuntime = runtimeId
      ;(window as unknown as { perchPreferredModel?: string }).perchPreferredModel = modelId
      setActiveRuntime(runtimeId)
      setActiveModel(modelId)
      return
    }
    if (runtimeId !== activeRuntime || modelId !== activeModel) {
      const ok = confirm('Switching agents starts a fresh agent context. The new runtime will not see prior turns. Continue?')
      if (!ok) return
    }
    const r = await fetch(`/api/conversations/${conversationId}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ runtime: runtimeId, model: modelId }),
    })
    if (!r.ok) {
      alert('Failed to switch runtime: ' + r.statusText)
      return
    }
    setActiveRuntime(runtimeId)
    setActiveModel(modelId)
  }

  const activeR = runtimes.find(r => r.id === activeRuntime)
  const modelOptions = activeR?.models ?? []

  return (
    <div style={{
      display: 'flex', alignItems: 'center', gap: 8,
      padding: '8px 16px', borderBottom: '1px solid #2a2a2a',
      background: '#1a1a1a', color: '#bbb', fontFamily: FONT, fontSize: 12,
    }}>
      <select
        value={activeRuntime}
        onChange={e => {
          const rt = e.target.value
          const r = runtimes.find(rr => rr.id === rt)
          onChange(rt, r?.default_model ?? '')
        }}
        style={{
          background: '#262626', border: '1px solid #333',
          color: '#ddd', borderRadius: 6, padding: '4px 8px',
        }}
        aria-label="runtime"
      >
        <option value="">— runtime —</option>
        {runtimes.map(r => (
          <option key={r.id} value={r.id}>{r.name}</option>
        ))}
      </select>
      <select
        value={activeModel}
        onChange={e => onChange(activeRuntime, e.target.value)}
        disabled={!activeR}
        style={{
          background: '#262626', border: '1px solid #333',
          color: '#ddd', borderRadius: 6, padding: '4px 8px',
        }}
        aria-label="model"
      >
        <option value="">— model —</option>
        {modelOptions.map(m => (
          <option key={m} value={m}>{m}</option>
        ))}
      </select>
      {activeR && !activeR.supports_mcp && (
        <span style={{ color: '#888', fontSize: 11 }} title="Agent cannot self-schedule on this runtime">
          (no agent tools)
        </span>
      )}
      <div style={{ flex: 1 }} />
      <button
        onClick={onScheduleClick}
        disabled={!conversationId}
        title="Schedules"
        aria-label="schedules"
        style={{
          background: '#262626', border: '1px solid #333',
          color: conversationId ? '#ddd' : '#555',
          borderRadius: 6, padding: '4px 10px',
          cursor: conversationId ? 'pointer' : 'not-allowed',
          fontSize: 14,
        }}
      >🕒</button>
    </div>
  )
}
