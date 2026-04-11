import { render, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { Terminal } from '@xterm/xterm'
import { Keyboard } from './Keyboard'

describe('Keyboard', () => {
  let inputFn: ReturnType<typeof vi.fn>
  let termRef: { current: Partial<Terminal> | null }

  beforeEach(() => {
    inputFn = vi.fn()
    termRef = { current: { input: inputFn } }
  })

  it.each([
    ['Tab',    '\t'],
    ['Ctrl+C', '\x03'],
    ['Ctrl+D', '\x04'],
    ['Ctrl+Z', '\x1a'],
    ['Esc',    '\x1b'],
    ['↑',     '\x1b[A'],
    ['↓',     '\x1b[B'],
    ['←',     '\x1b[D'],
    ['→',     '\x1b[C'],
  ])('%s sends correct sequence via input()', (label, seq) => {
    const { getByText } = render(<Keyboard termRef={termRef as React.RefObject<Terminal | null>} />)
    fireEvent.pointerDown(getByText(label))
    expect(inputFn).toHaveBeenCalledWith(seq, true)
  })

  it('calls preventDefault on pointerDown to prevent focus stealing', () => {
    const { getByText } = render(<Keyboard termRef={termRef as React.RefObject<Terminal | null>} />)
    const event = new PointerEvent('pointerdown', { bubbles: true, cancelable: true })
    const spy = vi.spyOn(event, 'preventDefault')
    getByText('Tab').dispatchEvent(event)
    expect(spy).toHaveBeenCalled()
  })

  it('does not call input() when termRef.current is null', () => {
    termRef.current = null
    const { getByText } = render(<Keyboard termRef={termRef as React.RefObject<Terminal | null>} />)
    expect(() => fireEvent.pointerDown(getByText('Tab'))).not.toThrow()
    expect(inputFn).not.toHaveBeenCalled()
  })

  it('collapses on ▼ click hiding key buttons', () => {
    const { getByText, queryByText } = render(<Keyboard termRef={termRef as React.RefObject<Terminal | null>} />)
    expect(queryByText('Tab')).not.toBeNull()
    fireEvent.click(getByText('▼'))
    expect(queryByText('Tab')).toBeNull()
  })

  it('re-expands on ⌨ click restoring key buttons', () => {
    const { getByText, queryByText } = render(<Keyboard termRef={termRef as React.RefObject<Terminal | null>} />)
    fireEvent.click(getByText('▼'))
    expect(queryByText('Tab')).toBeNull()
    fireEvent.click(getByText('⌨'))
    expect(queryByText('Tab')).not.toBeNull()
  })
})
