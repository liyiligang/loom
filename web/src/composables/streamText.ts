const segmenter = new Intl.Segmenter(undefined, { granularity: 'grapheme' })
const running = (status: string) => ['running', 'inProgress', 'streaming'].includes(status)

// A short presentation buffer only. The server remains authoritative for text and status.
export class StreamText {
  text: string
  private source: string
  private status: string
  private deadline = 0
  private tailStart = 0

  constructor(text: string, status: string) {
    this.text = this.source = text
    this.status = status
    this.rememberTail()
  }

  get pending() { return this.text !== this.source }

  update(text: string, status: string, now: number, animate = true) {
    const append = text.startsWith(this.source)
    const live = running(status) || (running(this.status) && status === 'completed')
    this.source = text
    this.status = status
    if (!animate || !append || !live || text.length > 48_000 || text.length - this.text.length > 2_048) {
      this.flush()
    } else if (this.pending) {
      if (!this.deadline) this.deadline = now + 180
      if (status === 'completed') this.deadline = Math.min(this.deadline, now + 120)
    }
    return this.text
  }

  advance(now: number) {
    if (!this.pending) return this.text
    if (now >= this.deadline) return this.flush()
    const remaining = this.source.length - this.text.length
    const desired = this.text.length + Math.max(1, Math.ceil(remaining * 32 / Math.max(32, this.deadline - now)))
    for (const part of segmenter.segment(this.source.slice(this.tailStart))) {
      const start = this.tailStart + part.index
      const end = start + part.segment.length
      if (end >= desired) {
        this.text = this.source.slice(0, end)
        this.tailStart = start
        break
      }
    }
    if (!this.pending) this.deadline = 0
    return this.text
  }

  flush() {
    this.text = this.source
    this.deadline = 0
    this.rememberTail()
    return this.text
  }

  private rememberTail() {
    // containing() avoids scanning a long history to find its last grapheme.
    this.tailStart = segmenter.segment(this.text).containing(this.text.length - 1)?.index ?? 0
  }
}
