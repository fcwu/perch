import { useState, useRef, useCallback, useEffect } from 'react'
import { ToolPanel, ToolEntry } from './ToolPanel'

function SendIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
      <line x1="22" y1="2" x2="11" y2="13"/>
      <polygon points="22 2 15 22 11 13 2 9 22 2"/>
    </svg>
  )
}

// Strip ANSI/VT100 escape sequences applied to the FULL accumulated buffer.
// OSC/DCS are tried before standalone Fe so \x1b] is not greedily consumed as a 2-char sequence.
// Standalone Fe excludes ], ^, _, P which are string-sequence introducers (OSC/DCS/PM/APC).
const ANSI_RE = /\x1B(?:\][^\x07\x1b]*(?:\x07|\x1B\\)|P[^\x1b]*(?:\x1b\\)?|\[(?:[0-9;:<=>?]|[ -/])*[@-~]|[@-OQ-Z\\])/g
function stripAnsi(s: string): string {
  return s.replace(ANSI_RE, '').replace(/\r\n/g, '\n').replace(/\r/g, '\n')
}

interface Message {
  role: 'user' | 'assistant'
  content: string
  done: boolean
}

interface ChatPageProps {
  userID: string
  conversationId?: string
  onSidebarToggle?: () => void
  sidebarOpen?: boolean
}

export default function ChatPage({ conversationId }: ChatPageProps) {
  const [query, setQuery] = useState('')
  const [messages, setMessages] = useState<Message[]>([])
  const [loading, setLoading] = useState(false)
  const [toolEntries, setToolEntries] = useState<ToolEntry[]>([])
  const esRef = useRef<EventSource | null>(null)
  const toolCounter = useRef(0)
  const threadRef = useRef<HTMLDivElement>(null)
  const rawBufRef = useRef('')

  // Auto-scroll to bottom after each new message or content update
  useEffect(() => {
    if (threadRef.current) {
      threadRef.current.scrollTop = threadRef.current.scrollHeight
    }
  }, [messages])

  const handleSubmit = useCallback(async () => {
    const q = query.trim()
    if (!q || loading) return
    setLoading(true)
    setToolEntries([])
    setQuery('')
    toolCounter.current = 0
    rawBufRef.current = ''

    // Append user message and a pending assistant slot
    setMessages(prev => [...prev, { role: 'user', content: q, done: true }, { role: 'assistant', content: '', done: false }])

    try {
      const resp = await fetch('/api/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ query: q, conversation_id: conversationId }),
      })
      if (resp.status === 409) {
        alert('A session is already in progress. Please wait.')
        setLoading(false)
        setMessages(prev => prev.slice(0, -2))
        return
      }
      if (!resp.ok) {
        alert('Failed to start session: ' + resp.statusText)
        setLoading(false)
        setMessages(prev => prev.slice(0, -2))
        return
      }
      const data = await resp.json()
      if (data.conversation_id && !conversationId) {
        // Update URL without reload and refresh sidebar
        window.history.replaceState({}, '', `/chat?id=${data.conversation_id}`)
        window.dispatchEvent(new CustomEvent('perch:refresh-conversations'))
      }
    } catch (e) {
      alert('Network error: ' + e)
      setLoading(false)
      setMessages(prev => prev.slice(0, -2))
      return
    }

    const es = new EventSource('/api/chat/stream')
    esRef.current = es

    const decoder = new TextDecoder()

    es.addEventListener('pty', (e: MessageEvent) => {
      const raw = atob(e.data)
      const bytes = new Uint8Array(raw.length)
      for (let i = 0; i < raw.length; i++) bytes[i] = raw.charCodeAt(i)
      rawBufRef.current += decoder.decode(bytes)
      const content = stripAnsi(rawBufRef.current)
      // Update the last (pending) assistant message in-place
      setMessages(prev => {
        const updated = [...prev]
        const last = updated[updated.length - 1]
        if (last && last.role === 'assistant') {
          updated[updated.length - 1] = { ...last, content }
        }
        return updated
      })
    })

    es.addEventListener('json', (e: MessageEvent) => {
      try {
        const ev = JSON.parse(e.data) as { type: string; tool?: string; input?: unknown; elapsed_ms?: number }
        if (ev.type === 'tool_start' && ev.tool) {
          const id = ++toolCounter.current
          const inputStr = ev.input ? JSON.stringify(ev.input) : ''
          setToolEntries(prev => [...prev, { id, tool: ev.tool!, input: inputStr, startTime: Date.now(), done: false }])
        } else if (ev.type === 'tool_end' && ev.tool) {
          setToolEntries(prev => prev.map(e =>
            e.tool === ev.tool && !e.done
              ? { ...e, done: true, elapsedMs: ev.elapsed_ms }
              : e
          ))
        } else if (ev.type === 'done') {
          // Mark last assistant message as complete
          setMessages(prev => {
            const updated = [...prev]
            const last = updated[updated.length - 1]
            if (last && last.role === 'assistant') {
              updated[updated.length - 1] = { ...last, done: true }
            }
            return updated
          })
          setLoading(false)
          setTimeout(() => es.close(), 300)
        }
      } catch { /* ignore malformed JSON */ }
    })

    es.onerror = () => {
      setLoading(false)
      es.close()
    }
  }, [query, loading, conversationId])

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSubmit()
    }
  }

  const FONT = "-apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif"

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100dvh', background: '#171717', color: '#e0e0e0', fontFamily: FONT }}>
      {/* Thread area */}
      <div ref={threadRef} style={{ flex: 1, overflowY: 'auto', padding: '24px 24px 8px' }}>
        {messages.length === 0 && !loading && (
          <div style={{ color: '#3a3a3a', textAlign: 'center', marginTop: '20vh', fontSize: 22, fontWeight: 500, letterSpacing: '-0.02em' }}>
            Ask anything
          </div>
        )}

        <div style={{ maxWidth: 740, margin: '0 auto' }}>
          {messages.map((msg, i) => (
            <div key={i} style={{ marginBottom: 20 }}>
              {msg.role === 'user' ? (
                <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
                  <div style={{
                    background: '#2a2a2a', color: '#e0e0e0', borderRadius: '16px 16px 4px 16px',
                    padding: '10px 16px', maxWidth: '72%', fontSize: 14, lineHeight: 1.6,
                    whiteSpace: 'pre-wrap', wordBreak: 'break-word',
                    border: '1px solid #333',
                  }}>
                    {msg.content}
                  </div>
                </div>
              ) : (
                <div style={{ display: 'flex', justifyContent: 'flex-start' }}>
                  <div style={{ maxWidth: '100%', width: '100%' }}>
                    {msg.content ? (
                      <pre style={{
                        margin: 0, lineHeight: 1.65,
                        whiteSpace: 'pre-wrap', wordBreak: 'break-word',
                        fontFamily: "'Menlo', 'Consolas', 'Monaco', monospace", fontSize: 13, color: '#d4d4d4',
                      }}>
                        {msg.content}
                      </pre>
                    ) : (
                      i === messages.length - 1 && !msg.done && (
                        <span style={{ color: '#555', fontSize: 13 }}>Thinking…</span>
                      )
                    )}
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      </div>

      {/* Activity bar (tool calls) */}
      <ToolPanel entries={toolEntries} loading={loading} />

      {/* Input area */}
      <div style={{ padding: '12px 24px 20px' }}>
        <div style={{ maxWidth: 740, margin: '0 auto' }}>
          <div style={{
            display: 'flex', alignItems: 'flex-end', gap: 0,
            background: '#1e1e1e', border: '1px solid #2e2e2e',
            borderRadius: 12, overflow: 'hidden',
            boxShadow: '0 0 0 1px rgba(0,0,0,0.3)',
          }}>
            <textarea
              value={query}
              onChange={e => setQuery(e.target.value)}
              onKeyDown={handleKeyDown}
              disabled={loading}
              placeholder="Message…"
              rows={1}
              style={{
                flex: 1,
                background: 'transparent',
                border: 'none',
                outline: 'none',
                color: '#e0e0e0',
                padding: '12px 14px',
                fontSize: 14,
                resize: 'none',
                fontFamily: FONT,
                lineHeight: 1.5,
                minHeight: 44,
                maxHeight: 180,
                opacity: loading ? 0.5 : 1,
              }}
            />
            <button
              onClick={handleSubmit}
              disabled={loading || !query.trim()}
              style={{
                background: query.trim() && !loading ? '#4a9eff' : 'transparent',
                border: 'none',
                borderRadius: 8,
                margin: 6,
                color: query.trim() && !loading ? '#fff' : '#3a3a3a',
                width: 32, height: 32,
                cursor: loading || !query.trim() ? 'not-allowed' : 'pointer',
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                flexShrink: 0,
                transition: 'background 0.15s, color 0.15s',
              }}
              title="Send"
            >
              <SendIcon />
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
