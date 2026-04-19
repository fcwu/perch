import { useState, useRef, useCallback, useEffect } from 'react'
import { ToolPanel, ToolEntry } from './ToolPanel'

// Strip ANSI/VT100 escape sequences including private-parameter sequences (CSI <=>? prefixes).
const ANSI_RE = /\x1B(?:[@-Z\\-_]|\[[0-9;:<=>?]*[ -/]*[@-~]|\][^\x07]*(?:\x07|\x1B\\))/g
function stripAnsi(s: string): string {
  return s.replace(ANSI_RE, '').replace(/\r\n/g, '\n').replace(/\r/g, '\n')
}

interface ChatPageProps {
  userID: string
}

export default function ChatPage(_: ChatPageProps) {
  const [query, setQuery] = useState('')
  const [submittedQuery, setSubmittedQuery] = useState('')
  const [loading, setLoading] = useState(false)
  const [markdownBuf, setMarkdownBuf] = useState('')
  const [toolEntries, setToolEntries] = useState<ToolEntry[]>([])
  const esRef = useRef<EventSource | null>(null)
  const toolCounter = useRef(0)
  const outputRef = useRef<HTMLDivElement>(null)

  // Auto-scroll output to bottom
  useEffect(() => {
    if (outputRef.current) {
      outputRef.current.scrollTop = outputRef.current.scrollHeight
    }
  }, [markdownBuf])

  const handleSubmit = useCallback(async () => {
    const q = query.trim()
    if (!q || loading) return
    setLoading(true)
    setMarkdownBuf('')
    setToolEntries([])
    setSubmittedQuery(q)
    setQuery('')
    toolCounter.current = 0

    try {
      const resp = await fetch('/api/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ query: q }),
      })
      if (resp.status === 409) {
        alert('A session is already in progress. Please wait.')
        setLoading(false)
        return
      }
      if (!resp.ok) {
        alert('Failed to start session: ' + resp.statusText)
        setLoading(false)
        return
      }
    } catch (e) {
      alert('Network error: ' + e)
      setLoading(false)
      return
    }

    const es = new EventSource('/api/chat/stream')
    esRef.current = es

    const decoder = new TextDecoder()

    es.addEventListener('pty', (e: MessageEvent) => {
      const raw = atob(e.data)
      const bytes = new Uint8Array(raw.length)
      for (let i = 0; i < raw.length; i++) bytes[i] = raw.charCodeAt(i)
      const text = stripAnsi(decoder.decode(bytes))
      setMarkdownBuf(prev => prev + text)
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
          setLoading(false)
          es.close()
        }
      } catch { /* ignore malformed JSON */ }
    })

    es.onerror = () => {
      setLoading(false)
      es.close()
    }
  }, [query, loading])

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSubmit()
    }
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100dvh', background: '#1a1a1a', color: '#e0e0e0', fontFamily: 'sans-serif' }}>
      {/* Response area */}
      <div ref={outputRef} style={{ flex: 1, overflowY: 'auto', padding: '20px 24px' }}>
        {!submittedQuery && !loading && (
          <div style={{ color: '#555', textAlign: 'center', marginTop: 80, fontSize: 18 }}>
            Ask anything about the knowledge base
          </div>
        )}

        {submittedQuery && (
          <div style={{ maxWidth: 800, margin: '0 auto 20px' }}>
            {/* User question bubble */}
            <div style={{ display: 'flex', justifyContent: 'flex-end', marginBottom: 16 }}>
              <div style={{
                background: '#2c4a7a', color: '#e0e0e0', borderRadius: '12px 12px 2px 12px',
                padding: '10px 16px', maxWidth: '75%', fontSize: 14, lineHeight: 1.5,
                whiteSpace: 'pre-wrap', wordBreak: 'break-word',
              }}>
                {submittedQuery}
              </div>
            </div>

            {/* Response */}
            {(markdownBuf || loading) && (
              <div style={{ display: 'flex', justifyContent: 'flex-start' }}>
                <div style={{ maxWidth: '100%', width: '100%' }}>
                  {markdownBuf ? (
                    <pre style={{
                      margin: 0, lineHeight: 1.6,
                      whiteSpace: 'pre-wrap', wordBreak: 'break-word',
                      fontFamily: 'monospace', fontSize: 13, color: '#e0e0e0',
                    }}>
                      {markdownBuf}
                    </pre>
                  ) : (
                    <span style={{ color: '#888', fontSize: 13 }}>⟳ Thinking…</span>
                  )}
                </div>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Activity bar (tool calls) */}
      <ToolPanel entries={toolEntries} loading={loading} />

      {/* Input area */}
      <div style={{ padding: '12px 16px', background: '#111', borderTop: '1px solid #333', display: 'flex', gap: 8 }}>
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
          }}
        >
          Send
        </button>
      </div>
    </div>
  )
}
