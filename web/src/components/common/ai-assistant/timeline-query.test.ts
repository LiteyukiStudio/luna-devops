import type { AIEvent, AITimeline, AITimelineItem, AITimelineTurn } from '@/api'
import { describe, expect, it, vi } from 'vitest'
import {
  activeRunStreamSubscriptions,
  applyTimelineInfiniteEvent,
  applyTimelineQueryEvent,
  mergeLatestTimelineSnapshot,
  mergeTimelineQuerySnapshot,
  recoverTimelineOnce,
  runStreamRecoveryFromTimeline,
  timelineQueryDataFromInfinite,
  timelineQueryDataFromSnapshot,
} from './timeline-query'

function snapshot(after = 0, item?: AITimelineItem): AITimeline {
  return {
    conversation: { id: 'conversation-1', title: '诊断', titleSource: 'assistant', status: 'active' },
    eventCursors: [{ runId: 'run-1', after }],
    pageInfo: { hasOlder: false },
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
    contentPartId: `${item.id}:0`,
    occurredAt: '2026-08-15T00:00:01Z',
    payload: {
      itemId: item.id,
      contentPartId: `${item.id}:0`,
      partIndex: 0,
      delta: item.parts[0]?.text ?? '',
      timelineIndex: item.timelineIndex,
      createdAt: item.createdAt,
    },
  }
}

function thinkingEvent(sequence: number, type: 'thinking.started' | 'thinking.delta', delta: string): AIEvent {
  return {
    version: 2,
    eventId: `thinking-event-${sequence}`,
    eventSequence: sequence,
    type,
    conversationId: 'conversation-1',
    turnId: 'turn-1',
    runId: 'run-1',
    itemId: 'thinking-1',
    occurredAt: '2026-08-15T00:00:01Z',
    payload: type === 'thinking.started'
      ? { itemId: 'thinking-1', summary: delta, display: 'summary', timelineIndex: 0, createdAt: '2026-08-15T00:00:01Z' }
      : { itemId: 'thinking-1', delta, display: 'summary', timelineIndex: 0 },
  }
}

function completedTurn(turnIndex: number): AITimelineTurn {
  return {
    id: `turn-${turnIndex}`,
    turnIndex,
    status: 'completed',
    input: {
      id: `turn-${turnIndex}:input`,
      type: 'user_message',
      createdAt: new Date(Date.UTC(2026, 7, 15, 0, turnIndex + 30)).toISOString(),
      parts: [{ id: `turn-${turnIndex}:input:0`, partIndex: 0, type: 'text', text: `message-${turnIndex}` }],
    },
  }
}

function timelinePage(turnIndexes: number[], pageInfo: AITimeline['pageInfo']) {
  return timelineQueryDataFromSnapshot({
    conversation: { id: 'conversation-1', title: '诊断', titleSource: 'assistant', status: 'active' },
    eventCursors: [],
    pageInfo,
    turns: turnIndexes.map(completedTurn),
  })
}

describe('timeline query cache', () => {
  it('keeps an already displayed turn when the latest window advances before history was loaded', () => {
    const current = {
      pageParams: [null],
      pages: [timelinePage(Array.from({ length: 30 }, (_, index) => index + 1), { hasOlder: true, olderCursor: 'before-1' })],
    }
    const merged = mergeLatestTimelineSnapshot(
      current,
      timelinePage(Array.from({ length: 30 }, (_, index) => index + 2), { hasOlder: true, olderCursor: 'before-2' }),
    )

    expect(merged.pages[0]?.snapshot?.turns.map(turn => turn.turnIndex)).toEqual(Array.from({ length: 31 }, (_, index) => index + 1))
    expect(merged.pages[0]?.snapshot?.pageInfo).toEqual({ hasOlder: true, olderCursor: 'before-1' })
  })

  it('preserves a turn displaced from the growing latest window without changing the older cursor', () => {
    const current = {
      pageParams: ['before-1', null],
      pages: [
        timelinePage(Array.from({ length: 24 }, (_, index) => index - 23), { hasOlder: true, olderCursor: 'before--23' }),
        timelinePage(Array.from({ length: 25 }, (_, index) => index + 1), { hasOlder: true, olderCursor: 'before-1' }),
      ],
    }
    const merged = mergeLatestTimelineSnapshot(
      current,
      timelinePage(Array.from({ length: 25 }, (_, index) => index + 2), { hasOlder: true, olderCursor: 'before-2' }),
    )
    const aggregate = timelineQueryDataFromInfinite(merged)

    expect(aggregate?.snapshot?.turns.map(turn => turn.turnIndex)).toEqual(Array.from({ length: 50 }, (_, index) => index - 23))
    expect(new Set(aggregate?.snapshot?.turns.map(turn => turn.id)).size).toBe(50)
    expect(merged.pages[0]?.snapshot?.pageInfo).toEqual({ hasOlder: true, olderCursor: 'before--23' })
  })

  it('keeps newer streamed revisions when a delayed snapshot replaces query data', () => {
    const initial = timelineQueryDataFromSnapshot(snapshot(0, messageItem(1, 'snapshot')))
    const streamed = applyTimelineQueryEvent(initial, event(1, messageItem(2, 'streamed')))
    const merged = mergeTimelineQuerySnapshot(streamed, timelineQueryDataFromSnapshot(snapshot(0, messageItem(1, 'stale'))))

    expect(merged.state.blocks.at(-1)).toMatchObject({ text: 'snapshotstreamed' })
    expect(merged.snapshot?.turns[0]?.selectedRun?.items[0]).toMatchObject({ revision: 1, parts: [{ text: 'stale' }] })
  })

  it('preserves a live-only message across an active refetch so a delta without createdAt can continue', () => {
    const first = applyTimelineQueryEvent(timelineQueryDataFromSnapshot(snapshot(3)), event(4, messageItem(1, 'Hel')))
    const refreshed = mergeTimelineQuerySnapshot(first, timelineQueryDataFromSnapshot(snapshot(4)))
    const continuation = event(5, messageItem(2, 'lo'))
    continuation.payload = {
      itemId: 'message-1',
      contentPartId: 'message-1:0',
      partIndex: 0,
      delta: 'lo',
      timelineIndex: 0,
    }
    const continued = applyTimelineQueryEvent(refreshed, continuation)

    expect(refreshed.state.blocks.find(block => block.id === 'message-1')).toMatchObject({ runId: 'run-1', text: 'Hel' })
    expect(continued.state.blocks.find(block => block.id === 'message-1')).toMatchObject({ text: 'Hello' })
    expect(continued.state.lastEventSequences['run-1']).toBe(5)
  })

  it('preserves live-only thinking across an active refetch so the next thinking delta can continue', () => {
    const first = applyTimelineQueryEvent(timelineQueryDataFromSnapshot(snapshot(3)), thinkingEvent(4, 'thinking.started', '先分析'))
    const refreshed = mergeTimelineQuerySnapshot(first, timelineQueryDataFromSnapshot(snapshot(4)))
    const continued = applyTimelineQueryEvent(refreshed, thinkingEvent(5, 'thinking.delta', '再检查'))

    expect(refreshed.state.blocks.find(block => block.id === 'thinking-1')).toMatchObject({ runId: 'run-1', text: '先分析' })
    expect(continued.state.blocks.find(block => block.id === 'thinking-1')).toMatchObject({ text: '先分析再检查' })
    expect(continued.state.lastEventSequences['run-1']).toBe(5)
  })

  it('drops live-only blocks and their revisions once the authoritative run is terminal', () => {
    const first = applyTimelineQueryEvent(timelineQueryDataFromSnapshot(snapshot(3)), event(4, messageItem(1, 'temporary')))
    const terminal = snapshot(5)
    terminal.turns[0]!.status = 'completed'
    terminal.turns[0]!.selectedRun!.status = 'completed'
    const merged = mergeTimelineQuerySnapshot(first, timelineQueryDataFromSnapshot(terminal))

    expect(merged.state.blocks.find(block => block.id === 'message-1')).toBeUndefined()
    expect(merged.state.itemRevisions['message-1']).toBeUndefined()
    expect(merged.state.runStatuses['run-1']).toBe('completed')
  })

  it('does not carry a live-only block into a replacement run for the same turn', () => {
    const first = applyTimelineQueryEvent(timelineQueryDataFromSnapshot(snapshot(3)), event(4, messageItem(1, 'old run')))
    const replacement = snapshot(0)
    replacement.eventCursors = [{ runId: 'run-2', after: 0 }]
    replacement.turns[0]!.selectedRun = {
      id: 'run-2',
      runIndex: 1,
      status: 'running',
      items: [],
    }
    const merged = mergeTimelineQuerySnapshot(first, timelineQueryDataFromSnapshot(replacement))

    expect(merged.state.blocks.find(block => block.id === 'message-1')).toBeUndefined()
    expect(merged.state.itemRevisions['message-1']).toBeUndefined()
    expect(merged.state.runStatuses['run-2']).toBe('running')
  })

  it('preserves cache identity for a duplicate event so side effects are not repeated', () => {
    const streamed = applyTimelineQueryEvent(timelineQueryDataFromSnapshot(snapshot()), event(1))

    expect(applyTimelineQueryEvent(streamed, event(1))).toBe(streamed)
  })

  it('preserves the latest context usage through infinite timeline aggregation', () => {
    const initial = {
      pageParams: [null],
      pages: [timelineQueryDataFromSnapshot(snapshot())],
    }
    const started = applyTimelineInfiniteEvent(initial, {
      ...event(1),
      type: 'run.started',
      item: undefined,
      payload: {},
    })
    const completed = applyTimelineInfiniteEvent(started, {
      ...event(2),
      type: 'model.completed',
      item: undefined,
      payload: { usage: { status: 'reported', promptTokens: 25_600, completionTokens: 512, totalTokens: 26_112 }, modelId: 'aimod_test', maxContextTokensSnapshot: 128_000 },
    })
    const aggregate = timelineQueryDataFromInfinite(completed)

    expect(aggregate?.state.runUsage['run-1']).toEqual({
      status: 'reported',
      promptTokens: 25_600,
      modelId: 'aimod_test',
      maxContextTokensSnapshot: 128_000,
    })
  })

  it('restores context usage from a durable timeline snapshot', () => {
    const durable = snapshot(8)
    durable.turns[0]!.selectedRun = {
      ...durable.turns[0]!.selectedRun!,
      latestPromptTokens: 32_000,
      latestUsageModelId: 'aimod_test',
      latestUsageMaxContextTokensSnapshot: 128_000,
    }
    const aggregate = timelineQueryDataFromInfinite({
      pageParams: [null],
      pages: [timelineQueryDataFromSnapshot(durable)],
    })

    expect(aggregate?.state.runUsage['run-1']).toEqual({
      status: 'reported',
      promptTokens: 32_000,
      modelId: 'aimod_test',
      maxContextTokensSnapshot: 128_000,
    })
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

  it('keeps an active gap recovery on the accepted cursor and converges after the terminal commit', () => {
    const subscription = {
      conversationId: 'conversation-1',
      eventsUrl: '/api/v1/ai/runs/run-1/events',
      runId: 'run-1',
      after: 1,
    }
    const active = timelineQueryDataFromSnapshot(snapshot(1, messageItem(1, 'accepted')))
    expect(runStreamRecoveryFromTimeline(active, subscription)).toEqual({ after: 1, terminal: false })

    const terminalSnapshot = snapshot(3, messageItem(3, 'committed'))
    terminalSnapshot.turns[0]!.status = 'completed'
    terminalSnapshot.turns[0]!.selectedRun!.status = 'completed'
    const terminal = timelineQueryDataFromSnapshot(terminalSnapshot)
    expect(runStreamRecoveryFromTimeline(terminal, subscription)).toEqual({ after: 3, terminal: true })
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
