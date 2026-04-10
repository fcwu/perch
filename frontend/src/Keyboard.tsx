import { RefObject, useState } from 'react'
import { Terminal } from '@xterm/xterm'

const KEYS: [string, string][] = [
  ['Tab', '\t'],
  ['Ctrl+C', '\x03'],
  ['Ctrl+D', '\x04'],
  ['Ctrl+Z', '\x1a'],
  ['Esc', '\x1b'],
  ['↑', '\x1b[A'],
  ['↓', '\x1b[B'],
  ['←', '\x1b[D'],
  ['→', '\x1b[C'],
]

interface Props {
  termRef: RefObject<Terminal | null>
}

export function Keyboard({ termRef }: Props) {
  const [collapsed, setCollapsed] = useState(false)

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
            termRef.current?.focus()
            termRef.current?.paste(seq)
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
