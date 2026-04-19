import { useState, useRef, useCallback, useEffect } from 'react'
import { ToolPanel, ToolEntry } from './ToolPanel'

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
}

export default function ChatPage(_: ChatPageProps) {
  const [query, setQuery] = useState('')
  const [messages, setMessages] = useState<Message[]>([])
  const [newConversation, setNewConversation] = useState(false)
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
    const sendNewConversation = newConversation
    setMessages(prev => {
      const base = sendNewConversation ? [] : prev
      return [...base, { role: 'user', content: q, done: true }, { role: 'assistant', content: '', done: false }]
    })
    setNewConversation(false)

    try {
      const resp = await fetch('/api/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ query: q, new_conversation: sendNewConversation }),
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
  }, [query, loading, newConversation])

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSubmit()
    }
  }

  const handleNewConversation = () => {
    setMessages([])
    setNewConversation(true)
    setToolEntries([])
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100dvh', background: '#1a1a1a', color: '#e0e0e0', fontFamily: 'sans-serif' }}>
      {/* Thread area */}
      <div ref={threadRef} style={{ flex: 1, overflowY: 'auto', padding: '20px 24px' }}>
        {messages.length === 0 && !loading && (
          <div style={{ color: '#555', textAlign: 'center', marginTop: 80, fontSize: 18 }}>
            Ask anything about the knowledge base
          </div>
        )}

        <div style={{ maxWidth: 800, margin: '0 auto' }}>
          {messages.map((msg, i) => (
            <div key={i} style={{ marginBottom: 16 }}>
              {msg.role === 'user' ? (
                <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
                  <div style={{
                    background: '#2c4a7a', color: '#e0e0e0', borderRadius: '12px 12px 2px 12px',
                    padding: '10px 16px', maxWidth: '75%', fontSize: 14, lineHeight: 1.5,
                    whiteSpace: 'pre-wrap', wordBreak: 'break-word',
                  }}>
                    {msg.content}
                  </div>
                </div>
              ) : (
                <div style={{ display: 'flex', justifyContent: 'flex-start' }}>
                  <div style={{ maxWidth: '100%', width: '100%' }}>
                    {msg.content ? (
                      <pre style={{
                        margin: 0, lineHeight: 1.6,
                        whiteSpace: 'pre-wrap', wordBreak: 'break-word',
                        fontFamily: 'monospace', fontSize: 13, color: '#e0e0e0',
                      }}>
                        {msg.content}
                      </pre>
                    ) : (
                      // Show "Thinking…" spinner only in the last pending assistant slot
                      i === messages.length - 1 && !msg.done && (
                        <span style={{ color: '#888', fontSize: 13 }}>⟳ Thinking…</span>
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
      <div style={{ padding: '12px 16px', background: '#111', borderTop: '1px solid #333', display: 'flex', gap: 8, alignItems: 'flex-end' }}>
        <button
          onClick={handleNewConversation}
          disabled={loading}
          title="Start a new conversation"
          style={{
            background: '#333',
            border: '1px solid #555',
            borderRadius: 6,
            color: '#aaa',
            padding: '8px 12px',
            cursor: loading ? 'not-allowed' : 'pointer',
            opacity: loading ? 0.5 : 1,
            fontSize: 13,
            whiteSpace: 'nowrap',
            height: 'fit-content',
          }}
        >
          New conversation
        </button>
        <textarea
          value={query}
          onChange={e => setQuery(e.target.value)}
          onKeyDown={handleKeyDown}
          disabled={loading}
          placeholder="Type your question and press Enter…"
          rows={2}
          style={{
            flex: 1,
            background: '#222',
            border: '1px solid #444',
            color: '#e0e0e0',
            borderRadius: 6,
            padding: '8px 12px',
            fontSize: 14,
            resize: 'none',
            fontFamily: 'sans-serif',
            opacity: loading ? 0.5 : 1,
          }}
        />
        <button
          onClick={handleSubmit}
          disabled={loading || !query.trim()}
          style={{
            background: '#4a9eff',
            border: 'none',
            borderRadius: 6,
            color: '#fff',
            padding: '0 16px',
            cursor: loading || !query.trim() ? 'not-allowed' : 'pointer',
            opacity: loading || !query.trim() ? 0.5 : 1,
            fontSize: 14,
            height: 'fit-content',
            alignSelf: 'flex-end',
            paddingTop: 10,
            paddingBottom: 10,
          }}
        >
          Send
        </button>
      </div>
    </div>
  )
}
