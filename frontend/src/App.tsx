import { useEffect, useRef } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { Keyboard } from './Keyboard'

// Matches http(s) URLs; stops at whitespace or control characters
const URL_RE = /https?:\/\/[^\s\x00-\x1f\x7f]+/g

export default function App() {
  const containerRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)

  useEffect(() => {
    const term = new Terminal({
      cursorBlink: true,
      fontFamily: 'monospace',
      fontSize: 14,
    })
    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)

    // Custom link provider that handles URLs wrapping across multiple terminal
    // rows. WebLinksAddon only matches within a single row; this uses
    // IBufferLine.isWrapped to reconstruct the original URL first.
    term.registerLinkProvider({
      provideLinks(bufferRow, callback) {
        const buffer = term.buffer.active
        // buffer.getLine() is 0-indexed; bufferRow is 1-indexed
        const getLine = (r: number) => buffer.getLine(r - 1)

        // Walk back to find the first row of this wrapped group
        let startRow = bufferRow
        while (startRow > 1 && getLine(startRow)?.isWrapped) startRow--

        // Collect all rows belonging to this group
        const rows: number[] = [startRow]
        for (let r = startRow; r <= buffer.length; r++) {
          const next = getLine(r + 1)
          if (!next?.isWrapped) break
          rows.push(r + 1)
        }

        if (!rows.includes(bufferRow)) { callback(undefined); return }

        // Get each row's text, trimming trailing spaces so that padding
        // characters don't break URL matching across wrapped lines.
        // (translateToString(false) pads rows to `cols` width; those spaces
        // would truncate the regex match at the first line boundary.)
        const trimmed = rows.map(r => (getLine(r)?.translateToString(false) ?? '').trimEnd())
        const combined = trimmed.join('')

        // Cumulative character lengths (for offset → coordinate mapping)
        const cumLen: number[] = [0]
        for (const s of trimmed) cumLen.push(cumLen[cumLen.length - 1] + s.length)

        // Map a character offset in `combined` back to buffer {x, y} coordinates
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
            activate: (_: MouseEvent, text: string) => window.open(text, '_blank'),
          })
        }
        callback(links.length ? links : undefined)
      },
    })

    term.open(containerRef.current!)
    fitAddon.fit()
    termRef.current = term

    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${location.host}/ws`)
    ws.binaryType = 'arraybuffer'

    ws.onopen = () => {
      fitAddon.fit()
      const { cols, rows } = term
      ws.send(JSON.stringify({ type: 'resize', cols, rows }))
    }

    ws.onmessage = (e) => {
      term.write(new Uint8Array(e.data as ArrayBuffer))
    }

    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data))
      }
    })

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

    // When the native mobile keyboard appears the visual viewport shrinks.
    // Resize the terminal to fit the remaining space and scroll to bottom so
    // the cursor line stays visible.
    const handleViewportResize = () => {
      sendResize()
      term.scrollToBottom()
    }
    window.visualViewport?.addEventListener('resize', handleViewportResize)

    return () => {
      observer.disconnect()
      window.removeEventListener('resize', sendResize)
      window.visualViewport?.removeEventListener('resize', handleViewportResize)
      ws.close()
      term.dispose()
    }
  }, [])

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100dvh', background: '#000' }}>
      <div ref={containerRef} style={{ flex: 1, overflow: 'hidden' }} />
      <Keyboard termRef={termRef} />
    </div>
  )
}
