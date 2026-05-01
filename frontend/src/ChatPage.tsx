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

function PaperclipIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M21.44 11.05l-9.19 9.19a6 6 0 01-8.49-8.49l9.19-9.19a4 4 0 015.66 5.66l-9.2 9.19a2 2 0 01-2.83-2.83l8.49-8.48"/>
    </svg>
  )
}

const ALLOWED_UPLOAD_MIME = ['image/png', 'image/jpeg', 'image/gif', 'image/webp']
const MAX_UPLOAD_BYTES_CLIENT = 10 * 1024 * 1024 // mirrors server default; server is authoritative
const MAX_UPLOAD_FILES_CLIENT = 4

interface PendingAttachment {
  filename: string
  mime_type: string
  size: number
  data_base64: string // raw base64 (no data: prefix)
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n}B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(0)}KB`
  return `${(n / (1024 * 1024)).toFixed(1)}MB`
}

async function fileToAttachment(file: File): Promise<PendingAttachment> {
  const buf = await file.arrayBuffer()
  const bytes = new Uint8Array(buf)
  let binary = ''
  for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i])
  return {
    filename: file.name || `pasted-${Date.now()}.png`,
    mime_type: file.type,
    size: file.size,
    data_base64: btoa(binary),
  }
}

// Strip ANSI/VT100 escape sequences applied to the FULL accumulated buffer.
// OSC/DCS are tried before standalone forms so \x1b] is not greedily consumed.
// The single-byte fallback covers Fe/Fp/Fs in 0x30-0x7E except the introducers
// (P=DCS, [=CSI, ]=OSC, ^=PM, _=APC), so DEC private 2-byte escapes like
// \x1b7 (DECSC) and \x1b8 (DECRC) — which claude emits when restoring the
// terminal on print-mode exit — are stripped instead of leaking "78".
const ANSI_RE = /\x1B(?:\][^\x07\x1b]*(?:\x07|\x1B\\)|P[^\x1b]*(?:\x1B\\)?|\[(?:[0-9;:<=>?]|[ -/])*[@-~]|[0-OQ-Z\\`a-z{|}~])/g
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
  const [attachments, setAttachments] = useState<PendingAttachment[]>([])
  const [dragOver, setDragOver] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const esRef = useRef<EventSource | null>(null)
  const toolCounter = useRef(0)
  const threadRef = useRef<HTMLDivElement>(null)
  const rawBufRef = useRef('')

  const addFiles = useCallback(async (files: FileList | File[]) => {
    const list = Array.from(files).filter(f => ALLOWED_UPLOAD_MIME.includes(f.type))
    if (list.length === 0) return
    const errors: string[] = []
    setAttachments(prev => {
      const room = MAX_UPLOAD_FILES_CLIENT - prev.length
      if (room <= 0) {
        errors.push(`Max ${MAX_UPLOAD_FILES_CLIENT} files`)
        return prev
      }
      return prev
    })
    const ready: PendingAttachment[] = []
    for (const f of list) {
      if (f.size > MAX_UPLOAD_BYTES_CLIENT) {
        errors.push(`${f.name}: ${formatBytes(f.size)} > ${formatBytes(MAX_UPLOAD_BYTES_CLIENT)}`)
        continue
      }
      ready.push(await fileToAttachment(f))
    }
    if (ready.length > 0) {
      setAttachments(prev => [...prev, ...ready].slice(0, MAX_UPLOAD_FILES_CLIENT))
    }
    if (errors.length > 0) alert('Upload skipped:\n' + errors.join('\n'))
  }, [])

  const removeAttachment = useCallback((idx: number) => {
    setAttachments(prev => prev.filter((_, i) => i !== idx))
  }, [])

  // Auto-scroll to bottom after each new message or content update
  useEffect(() => {
    if (threadRef.current) {
      threadRef.current.scrollTop = threadRef.current.scrollHeight
    }
  }, [messages])

  const handleSubmit = useCallback(async () => {
    const q = query.trim()
    if ((!q && attachments.length === 0) || loading) return
    setLoading(true)
    setToolEntries([])
    setQuery('')
    const sentAttachments = attachments
    setAttachments([])
    toolCounter.current = 0
    rawBufRef.current = ''

    // Display the user's message with image-attachment markers so history matches server placeholder.
    const userDisplay = sentAttachments.length > 0
      ? sentAttachments.map(a => `[image:${a.filename}]`).join(' ') + (q ? ' ' + q : '')
      : q
    setMessages(prev => [...prev, { role: 'user', content: userDisplay, done: true }, { role: 'assistant', content: '', done: false }])

    try {
      const body: Record<string, unknown> = { query: q, conversation_id: conversationId }
      if (sentAttachments.length > 0) {
        body.attachments = sentAttachments.map(a => ({
          filename: a.filename,
          mime_type: a.mime_type,
          data_base64: a.data_base64,
        }))
      }
      const resp = await fetch('/api/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
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
  }, [query, loading, conversationId, attachments])

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
          {/* Attachment chips */}
          {attachments.length > 0 && (
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginBottom: 8 }}>
              {attachments.map((a, i) => (
                <span key={i} style={{
                  display: 'inline-flex', alignItems: 'center', gap: 6,
                  background: '#2a2a2a', border: '1px solid #3a3a3a',
                  borderRadius: 6, padding: '4px 8px', fontSize: 12, color: '#cfcfcf',
                  fontFamily: FONT,
                }}>
                  📎 {a.filename} <span style={{ color: '#888' }}>{formatBytes(a.size)}</span>
                  <button
                    onClick={() => removeAttachment(i)}
                    title="Remove"
                    style={{
                      background: 'transparent', border: 'none', color: '#888',
                      cursor: 'pointer', padding: 0, lineHeight: 1, fontSize: 14,
                    }}
                  >✕</button>
                </span>
              ))}
            </div>
          )}
          <div
            style={{
              display: 'flex', alignItems: 'flex-end', gap: 0,
              background: '#1e1e1e', border: '1px solid ' + (dragOver ? '#4a9eff' : '#2e2e2e'),
              borderRadius: 12, overflow: 'hidden',
              boxShadow: '0 0 0 1px rgba(0,0,0,0.3)',
              transition: 'border-color 0.15s',
            }}
            onDragEnter={e => { e.preventDefault(); setDragOver(true) }}
            onDragOver={e => { e.preventDefault(); setDragOver(true) }}
            onDragLeave={e => { e.preventDefault(); setDragOver(false) }}
            onDrop={e => {
              e.preventDefault()
              setDragOver(false)
              if (e.dataTransfer.files.length > 0) addFiles(e.dataTransfer.files)
            }}
          >
            <input
              ref={fileInputRef}
              type="file"
              accept="image/*"
              multiple
              style={{ display: 'none' }}
              onChange={e => {
                if (e.target.files) addFiles(e.target.files)
                e.target.value = '' // allow re-selecting the same file
              }}
            />
            <button
              onClick={() => fileInputRef.current?.click()}
              disabled={loading || attachments.length >= MAX_UPLOAD_FILES_CLIENT}
              title="Attach images"
              style={{
                background: 'transparent', border: 'none', borderRadius: 8,
                margin: 6, color: '#888', width: 32, height: 32,
                cursor: loading || attachments.length >= MAX_UPLOAD_FILES_CLIENT ? 'not-allowed' : 'pointer',
                display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0,
              }}
            >
              <PaperclipIcon />
            </button>
            <textarea
              value={query}
              onChange={e => setQuery(e.target.value)}
              onKeyDown={handleKeyDown}
              onPaste={e => {
                const files: File[] = []
                for (const item of e.clipboardData.items) {
                  if (item.kind === 'file') {
                    const f = item.getAsFile()
                    if (f) {
                      // Pasted images often have empty name; fabricate one.
                      const named = f.name ? f : new File([f], `pasted-${Date.now()}.png`, { type: f.type })
                      files.push(named)
                    }
                  }
                }
                if (files.length > 0) {
                  e.preventDefault()
                  addFiles(files)
                }
              }}
              disabled={loading}
              placeholder={dragOver ? 'Drop images here…' : 'Message…'}
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
              disabled={loading || (!query.trim() && attachments.length === 0)}
              style={{
                background: (query.trim() || attachments.length > 0) && !loading ? '#4a9eff' : 'transparent',
                border: 'none',
                borderRadius: 8,
                margin: 6,
                color: (query.trim() || attachments.length > 0) && !loading ? '#fff' : '#3a3a3a',
                width: 32, height: 32,
                cursor: loading || (!query.trim() && attachments.length === 0) ? 'not-allowed' : 'pointer',
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
