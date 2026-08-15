import type { AIEvent, AITimeline, AITimelineItem } from '@/api'
import { describe, expect, it, vi } from 'vitest'
import {
  activeRunStreamSubscriptions,
  applyTimelineQueryEvent,
  mergeTimelineQuerySnapshot,
  recoverTimelineOnce,
  timelineQueryDataFromSnapshot,
} from './timeline-query'

function snapshot(after = 0, item?: AITimelineItem): AITimeline {
  return {
    conversation: { id: 'conversation-1', title: '诊断', titleSource: 'assistant', status: 'active' },
    eventCursors: [{ runId: 'run-1', after }],
    turns: [{
      id: 'turn-1',
      turnIndex: 0,
      status: 'running',
      input: {
        id: 'turn-1:input',
        type: 'user_message',
        createdAt: '2026-08-15T00:00:00Z',
        parts: [{ id: 'turn-1:input:0', partIndex: 0, type: 'text', text: '检查部署' }],
      },
      selectedRun: {
        id: 'run-1',
        runIndex: 0,
        status: 'running',
        items: item ? [item] : [],
      },
    }],
  }
}

function messageItem(revision: number, text: string): AITimelineItem {
  return {
    id: 'message-1',
    timelineIndex: 0,
    revision,
    createdAt: '2026-08-15T00:00:01Z',
    type: 'assistant_message',
    status: 'streaming',
    parts: [{ id: 'message-1:0', partIndex: 0, type: 'text', text }],
  }
}

function event(sequence: number, item = messageItem(sequence, `answer-${sequence}`)): AIEvent {
  return {
    version: 2,
    eventId: `event-${sequence}`,
    eventSequence: sequence,
    type: 'content.delta',
    conversationId: 'conversation-1',
    turnId: 'turn-1',
    runId: 'run-1',
    itemId: item.id,
    item,
    occurredAt: '2026-08-15T00:00:01Z',
    payload: {},
  }
}

describe('timeline query cache', () => {
  it('keeps newer streamed revisions when a delayed snapshot replaces query data', () => {
    const initial = timelineQueryDataFromSnapshot(snapshot(0, messageItem(1, 'snapshot')))
    const streamed = applyTimelineQueryEvent(initial, event(1, messageItem(2, 'streamed')))
    const merged = mergeTimelineQuerySnapshot(streamed, timelineQueryDataFromSnapshot(snapshot(0, messageItem(1, 'stale'))))

    expect(merged.state.blocks.at(-1)).toMatchObject({ text: 'streamed' })
    expect(merged.snapshot?.turns[0]?.selectedRun?.items[0]).toMatchObject({ revision: 1, parts: [{ text: 'stale' }] })
  })

  it('preserves cache identity for a duplicate event so side effects are not repeated', () => {
    const streamed = applyTimelineQueryEvent(timelineQueryDataFromSnapshot(snapshot()), event(1))

    expect(applyTimelineQueryEvent(streamed, event(1))).toBe(streamed)
  })

  it('marks a sequence gap and only clears it after an authoritative snapshot covers the missing event', () => {
    const initial = applyTimelineQueryEvent(timelineQueryDataFromSnapshot(snapshot()), event(1))
    const gap = applyTimelineQueryEvent(initial, event(3))

    expect(gap.state.desyncedRunIds.has('run-1')).toBe(true)
    expect(gap.state.blocks.at(-1)).toMatchObject({ text: 'answer-1' })

    const stillBehind = mergeTimelineQuerySnapshot(gap, timelineQueryDataFromSnapshot(snapshot(2, messageItem(2, 'answer-2'))))
    expect(stillBehind.state.desyncedRunIds.has('run-1')).toBe(true)

    const recovered = mergeTimelineQuerySnapshot(stillBehind, timelineQueryDataFromSnapshot(snapshot(3, messageItem(3, 'answer-3'))))
    expect(recovered.state.desyncedRunIds.has('run-1')).toBe(false)
    expect(recovered.state.blocks.at(-1)).toMatchObject({ text: 'answer-3' })
  })

  it('restores active subscriptions from the authoritative cache when local handles are absent', () => {
    const data = applyTimelineQueryEvent(timelineQueryDataFromSnapshot(snapshot(4)), event(5))

    expect(activeRunStreamSubscriptions(data)).toEqual([{
      conversationId: 'conversation-1',
      eventsUrl: '/api/v1/ai/runs/run-1/events',
      runId: 'run-1',
      after: 5,
    }])

    const completed = applyTimelineQueryEvent(data, { ...event(6), type: 'run.completed', item: undefined })
    expect(activeRunStreamSubscriptions(completed)).toEqual([])
  })

  it('retains a newly created run subscription until the refreshed snapshot contains it', () => {
    const data = timelineQueryDataFromSnapshot(snapshot())
    const optimistic = {
      conversationId: 'conversation-1',
      eventsUrl: '/api/v1/ai/runs/run-new/events',
      runId: 'run-new',
      after: 0,
    }

    expect(activeRunStreamSubscriptions(data, 'conversation-1', [optimistic])).toEqual([
      expect.objectContaining({ runId: 'run-1' }),
      optimistic,
    ])
  })

  it('coalesces concurrent snapshot recovery for the same conversation', async () => {
    const recoveries = new Set<string>()
    let resolve: (() => void) | undefined
    const recover = vi.fn(() => new Promise<void>((done) => {
      resolve = done
    }))

    const first = recoverTimelineOnce(recoveries, 'conversation-1', recover)
    const duplicate = recoverTimelineOnce(recoveries, 'conversation-1', recover)
    expect(recover).toHaveBeenCalledOnce()

    resolve?.()
    await Promise.all([first, duplicate])
    expect(recoveries.size).toBe(0)
  })
})
