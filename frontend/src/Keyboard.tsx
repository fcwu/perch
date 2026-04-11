import { RefObject, useState } from 'react'
import { Terminal } from '@xterm/xterm'

const KEYS: [string, string][] = [
  ['Esc', '\x1b'],
  ['↑', '\x1b[A'],
  ['↓', '\x1b[B'],
  ['←', '\x1b[D'],
  ['→', '\x1b[C'],
]

const isMobile =
  typeof window !== 'undefined' &&
  typeof window.matchMedia === 'function' &&
  window.matchMedia('(hover: none) and (pointer: coarse)').matches

const btn: React.CSSProperties = {
  padding: '6px 14px',
  fontFamily: 'monospace',
  fontSize: 14,
  cursor: 'pointer',
  background: '#1c1c1c',
  color: '#c8c8c8',
  border: '1px solid #3a3a3a',
  borderRadius: 5,
  flexShrink: 0,
  userSelect: 'none',
}

interface Props {
  termRef: RefObject<Terminal | null>
}

export function Keyboard({ termRef }: Props) {
  const [collapsed, setCollapsed] = useState(!isMobile)

  if (collapsed) {
    return (
      <button
        onClick={() => setCollapsed(false)}
        style={{ ...btn, position: 'fixed', bottom: 10, right: 10, zIndex: 10 }}
      >
        ⌨
      </button>
    )
  }

  return (
    <div style={{
      position: 'fixed',
      bottom: 0,
      left: 0,
      right: 0,
      display: 'flex',
      flexWrap: 'nowrap',
      alignItems: 'center',
      background: '#000',
      borderTop: '1px solid #2a2a2a',
      padding: '6px 8px',
      gap: 6,
      zIndex: 10,
    }}>
      {KEYS.map(([label, seq]) => (
        <button
          key={label}
          onPointerDown={(e) => {
            e.preventDefault()
            termRef.current?.input(seq, true)
            termRef.current?.scrollToBottom?.()
          }}
          style={btn}
        >
          {label}
        </button>
      ))}
      <button
        onClick={() => setCollapsed(true)}
        style={{ ...btn, marginLeft: 'auto', background: 'transparent', border: 'none', color: '#555' }}
      >
        ▼
      </button>
    </div>
  )
}
