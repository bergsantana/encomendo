import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { trapTab } from './focusTrap'

let container: HTMLElement
let first: HTMLButtonElement
let last: HTMLButtonElement

beforeEach(() => {
  container = document.createElement('div')
  container.tabIndex = -1
  container.innerHTML = '<button id="first">A</button><input id="middle"><button id="gone" disabled>X</button><a id="last" href="#x">B</a>'
  document.body.append(container)
  first = container.querySelector<HTMLButtonElement>('#first') as HTMLButtonElement
  last = container.querySelector<HTMLButtonElement>('#last') as unknown as HTMLButtonElement
})

afterEach(() => container.remove())

const press = (key: string, shiftKey = false) => {
  const event = new KeyboardEvent('keydown', { key, shiftKey, cancelable: true })
  trapTab(event, container)
  return event
}

describe('trapTab', () => {
  it('wraps Tab from the last focusable element to the first', () => {
    last.focus()

    const event = press('Tab')

    expect(event.defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(first)
  })

  it('wraps Shift+Tab from the first element, or from the container itself, to the last', () => {
    first.focus()
    expect(press('Tab', true).defaultPrevented).toBe(true)
    expect(document.activeElement).toBe(last)

    container.focus()
    press('Tab', true)
    expect(document.activeElement).toBe(last)
  })

  it('leaves Tab in the middle to the browser, and skips disabled controls', () => {
    const middle = container.querySelector<HTMLElement>('#middle')
    middle?.focus()

    const event = press('Tab')

    expect(event.defaultPrevented).toBe(false)
    expect(document.activeElement).toBe(middle)
  })

  it('ignores other keys and containers with nothing to focus', () => {
    last.focus()
    expect(press('Enter').defaultPrevented).toBe(false)
    expect(document.activeElement).toBe(last)

    const empty = document.createElement('div')
    const event = new KeyboardEvent('keydown', { key: 'Tab', cancelable: true })
    trapTab(event, empty)
    expect(event.defaultPrevented).toBe(false)
  })
})
