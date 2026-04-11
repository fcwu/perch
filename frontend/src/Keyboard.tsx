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

interface Props {
  termRef: RefObject<Terminal | null>
}

export function Keyboard({ termRef }: Props) {
  const [collapsed, setCollapsed] = useState(!isMobile)

  if (collapsed) {
    return (
      <button
        onClick={() => setCollapsed(false)}
        style={{ position: 'fixed', bottom: 8, right: 8, zIndex: 10, padding: '8px 12px' }}
      >
        ⌨
      </button>
    )
  }

  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', background: '#111', padding: 4, gap: 4 }}>
      {KEYS.map(([label, seq]) => (
        <button
          key={label}
          onPointerDown={(e) => {
            e.preventDefault()
            termRef.current?.input(seq, true)
          }}
          style={{ padding: '6px 10px', fontFamily: 'monospace', fontSize: 13, cursor: 'pointer' }}
        >
          {label}
        </button>
      ))}
      <button onClick={() => setCollapsed(true)} style={{ marginLeft: 'auto', padding: '6px 10px' }}>
        ▼
      </button>
    </div>
  )
}
