import { useEffect, useRef, useState, useCallback } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { Keyboard } from './Keyboard'

const URL_RE = /https?:\/\/[^\s\x00-\x1f\x7f]+/g

interface SessionView {
  channel_id: string
  session_uuid: string
}

type ActiveTab = null | string  // null = main PTY; string = discord channelID

function useDiscordSessions() {
  const [sessions, setSessions] = useState<SessionView[]>([])
  useEffect(() => {
    const poll = async () => {
      try {
        const r = await fetch('/sessions')
        if (r.ok) setSessions(await r.json())
      } catch { /* ignore network errors */ }
    }
    poll()
    const id = setInterval(poll, 5000)
    return () => clearInterval(id)
  }, [])
  return sessions
}

// Shared logout button style
function LogoutButton() {
  return (
    <button
      onClick={() => { window.location.href = '/auth/logout' }}
      style={{
        background: 'none', border: '1px solid #444', borderRadius: 4,
        color: '#888', padding: '4px 10px', fontSize: 12, cursor: 'pointer',
        fontFamily: 'monospace',
      }}
    >
      Logout
    </button>
  )
}

export default function TerminalApp({ showLogout }: { showLogout: boolean }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)
  const fitAddonRef = useRef<FitAddon | null>(null)
  const sessions = useDiscordSessions()
  const [activeTab, setActiveTab] = useState<ActiveTab>(null)

  // Initial terminal setup — runs once on mount
  useEffect(() => {
    const term = new Terminal({
      cursorBlink: true,
      fontFamily: 'monospace',
      fontSize: 14,
    })
    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)

    // Custom link provider that handles URLs wrapping across multiple terminal
    // rows. WebLinksAddon only matches within a single row; this handles two
    // cases: (1) isWrapped=true terminal wrapping, and (2) explicit \n splits
    // used by Claude Code for long OAuth URLs where each line has no whitespace.
    term.registerLinkProvider({
      provideLinks(bufferRow, callback) {
        const buffer = term.buffer.active
        const getLine = (r: number) => buffer.getLine(r - 1)
        const rowText = (r: number) => (getLine(r)?.translateToString(false) ?? '').trimEnd()
        const noSpace = (s: string) => s.length > 0 && !/\s/.test(s)

        let startRow = bufferRow
        while (startRow > 1) {
          if (getLine(startRow)?.isWrapped) { startRow--; continue }
          const t = rowText(startRow)
          if (noSpace(t) && !t.startsWith('http') && noSpace(rowText(startRow - 1))) {
            startRow--; continue
          }
          break
        }

        const rows: number[] = [startRow]
        for (let r = startRow; r <= buffer.length; r++) {
          const next = getLine(r + 1)
          if (!next) break
          if (next.isWrapped) {
            rows.push(r + 1)
          } else {
            const nextT = rowText(r + 1)
            if (noSpace(rowText(r)) && noSpace(nextT) && !nextT.startsWith('http')) {
              rows.push(r + 1)
            } else {
              break
            }
          }
        }

        if (!rows.includes(bufferRow)) { callback(undefined); return }

        const trimmed = rows.map(r => rowText(r))
        const combined = trimmed.join('')

        const cumLen: number[] = [0]
        for (const s of trimmed) cumLen.push(cumLen[cumLen.length - 1] + s.length)

        const toCoord = (off: number) => {
          let ri = trimmed.length - 1
          for (let i = 0; i < trimmed.length; i++) {
            if (off < cumLen[i + 1]) { ri = i; break }
          }
          return { y: rows[ri], x: (off - cumLen[ri]) + 1 }
        }

        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const links: any[] = []
        URL_RE.lastIndex = 0
        let m: RegExpExecArray | null
        while ((m = URL_RE.exec(combined)) !== null) {
          const uri = m[0]
          const s = m.index
          const e = s + uri.length - 1
          links.push({
            text: uri,
            range: { start: toCoord(s), end: toCoord(e) },
            decorations: { pointerCursor: true, underline: true },
            activate: (_: MouseEvent, text: string) => {
              const a = document.createElement('a')
              a.href = text
              a.target = '_blank'
              a.rel = 'noopener noreferrer'
              a.click()
            },
          })
        }
        callback(links.length ? links : undefined)
      },
    })

    term.open(containerRef.current!)
    fitAddon.fit()
    termRef.current = term
    fitAddonRef.current = fitAddon

    return () => {
      term.dispose()
      termRef.current = null
      fitAddonRef.current = null
    }
  }, [])

  const connectWS = useCallback((tab: ActiveTab) => {
    const term = termRef.current
    const fitAddon = fitAddonRef.current
    if (!term || !fitAddon || !containerRef.current) return

    term.reset()

    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    const url = tab === null
      ? `${proto}://${location.host}/ws`
      : `${proto}://${location.host}/ws/session?id=${encodeURIComponent(tab)}`

    const ws = new WebSocket(url)
    ws.binaryType = 'arraybuffer'

    ws.onopen = () => {
      fitAddon.fit()
      const { cols, rows } = term
      ws.send(JSON.stringify({ type: 'resize', cols, rows }))
    }

    ws.onmessage = (e) => {
      term.write(new Uint8Array(e.data as ArrayBuffer))
    }

    const disposable = term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data))
      }
    })
    const disposeOnData = () => disposable.dispose()

    const sendResize = () => {
      fitAddon.fit()
      const { cols, rows } = term
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols, rows }))
      }
    }

    const observer = new ResizeObserver(sendResize)
    observer.observe(containerRef.current!)
    window.addEventListener('resize', sendResize)

    const handleViewportResize = () => {
      sendResize()
      term.scrollToBottom()
    }
    window.visualViewport?.addEventListener('resize', handleViewportResize)

    return () => {
      observer.disconnect()
      window.removeEventListener('resize', sendResize)
      window.visualViewport?.removeEventListener('resize', handleViewportResize)
      disposeOnData?.()
      ws.close()
    }
  }, [])

  useEffect(() => {
    return connectWS(activeTab)
  }, [activeTab, connectWS])

  const tabStyle = (active: boolean): React.CSSProperties => ({
    padding: '4px 12px',
    cursor: 'pointer',
    background: active ? '#333' : '#111',
    color: active ? '#fff' : '#888',
    border: 'none',
    borderBottom: active ? '2px solid #4af' : '2px solid transparent',
    fontSize: 12,
    fontFamily: 'monospace',
    flexShrink: 0,
  })

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100dvh', background: '#000' }}>
      <div style={{ display: 'flex', background: '#111', flexShrink: 0, overflowX: 'auto', alignItems: 'center' }}>
        {sessions.length > 0 && (
          <>
            <button style={tabStyle(activeTab === null)} onClick={() => setActiveTab(null)}>
              Terminal
            </button>
            {sessions.map(sess => (
              <button
                key={sess.channel_id}
                style={tabStyle(activeTab === sess.channel_id)}
                onClick={() => setActiveTab(sess.channel_id)}
              >
                Discord {sess.channel_id}
              </button>
            ))}
          </>
        )}
        {showLogout && (
          <div style={{ marginLeft: 'auto', padding: '4px 8px' }}>
            <LogoutButton />
          </div>
        )}
      </div>
      <div ref={containerRef} style={{ flex: 1, overflow: 'hidden' }} />
      <Keyboard termRef={termRef} />
    </div>
  )
}
