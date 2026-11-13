type TaskKeyEvent = Pick<KeyboardEvent, 'key' | 'isComposing' | 'keyCode' | 'shiftKey' | 'altKey' | 'ctrlKey' | 'metaKey'>

export function shouldSendTask(event: TaskKeyEvent): boolean {
  return event.key === 'Enter' && !event.isComposing && event.keyCode !== 229
    && !event.shiftKey && !event.altKey && !event.ctrlKey && !event.metaKey
}
