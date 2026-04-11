// jsdom does not implement PointerEvent; provide a minimal polyfill so tests
// that create PointerEvent instances (e.g. to spy on preventDefault) work.
if (typeof PointerEvent === 'undefined') {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  ;(globalThis as any).PointerEvent = class PointerEvent extends MouseEvent {
    constructor(type: string, init?: PointerEventInit) {
      super(type, init)
    }
  }
}

// jsdom does not implement matchMedia; stub it so Keyboard.tsx mobile detection
// returns false (desktop mode) in tests.
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  }),
})
