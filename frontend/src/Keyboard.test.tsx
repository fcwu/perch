import { render, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { Terminal } from '@xterm/xterm'
import { Keyboard } from './Keyboard'

// jsdom: matchMedia returns matches=false → isMobile=false → keyboard starts collapsed
const expand = (getByText: (t: string) => HTMLElement) => fireEvent.click(getByText('⌨'))

describe('Keyboard', () => {
  let inputFn: ReturnType<typeof vi.fn>
  let termRef: { current: Partial<Terminal> | null }

  beforeEach(() => {
    inputFn = vi.fn()
    termRef = { current: { input: inputFn } }
  })

  it.each([
    ['Esc', '\x1b'],
    ['↑',   '\x1b[A'],
    ['↓',   '\x1b[B'],
    ['←',   '\x1b[D'],
    ['→',   '\x1b[C'],
  ])('%s sends correct sequence via input()', (label, seq) => {
    const { getByText } = render(<Keyboard termRef={termRef as React.RefObject<Terminal | null>} />)
    expand(getByText)
    fireEvent.pointerDown(getByText(label))
    expect(inputFn).toHaveBeenCalledWith(seq, true)
  })

  it('calls preventDefault on pointerDown to prevent focus stealing', () => {
    const { getByText } = render(<Keyboard termRef={termRef as React.RefObject<Terminal | null>} />)
    expand(getByText)
    const event = new PointerEvent('pointerdown', { bubbles: true, cancelable: true })
    const spy = vi.spyOn(event, 'preventDefault')
    getByText('Esc').dispatchEvent(event)
    expect(spy).toHaveBeenCalled()
  })

  it('does not call input() when termRef.current is null', () => {
    termRef.current = null
    const { getByText } = render(<Keyboard termRef={termRef as React.RefObject<Terminal | null>} />)
    expand(getByText)
    expect(() => fireEvent.pointerDown(getByText('Esc'))).not.toThrow()
    expect(inputFn).not.toHaveBeenCalled()
  })

  it('expands on ⌨ click showing key buttons', () => {
    const { getByText, queryByText } = render(<Keyboard termRef={termRef as React.RefObject<Terminal | null>} />)
    expect(queryByText('Esc')).toBeNull()
    fireEvent.click(getByText('⌨'))
    expect(queryByText('Esc')).not.toBeNull()
  })

  it('collapses on ▼ click hiding key buttons', () => {
    const { getByText, queryByText } = render(<Keyboard termRef={termRef as React.RefObject<Terminal | null>} />)
    expand(getByText)
    expect(queryByText('Esc')).not.toBeNull()
    fireEvent.click(getByText('▼'))
    expect(queryByText('Esc')).toBeNull()
  })
})
