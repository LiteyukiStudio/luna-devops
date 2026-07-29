import type { Conversation, CreatedTurn, CreateTurn, Run, RunEvent, TimelineItem } from "../domain.js"

export interface Repository {
  health(): Promise<boolean>
  createConversation(ownerUserId: string, title: string, projectId?: string): Promise<Conversation>
  listConversations(ownerUserId: string, page: number, pageSize: number): Promise<{ items: Conversation[], total: number }>
  getConversation(ownerUserId: string, conversationId: string): Promise<Conversation | undefined>
  renameConversation(ownerUserId: string, conversationId: string, title: string): Promise<Conversation | undefined>
  deleteConversation(ownerUserId: string, conversationId: string): Promise<boolean>
  createTurn(ownerUserId: string, input: CreateTurn): Promise<CreatedTurn>
  getRun(ownerUserId: string, runId: string): Promise<Run | undefined>
  cancelRun(ownerUserId: string, runId: string): Promise<Run | undefined>
  claimRun(instanceId: string, leaseSeconds: number): Promise<Run | undefined>
  getExecutionInput(runId: string): Promise<{ turnId: string, input: string, pageContext: Record<string, unknown>, toolResults: unknown[] } | undefined>
  getRunActorGrantCiphertext(runId: string): Promise<string | undefined>
  appendRunInput(runId: string, text: string): Promise<void>
  renewLease(runId: string, instanceId: string, leaseSeconds: number): Promise<boolean>
  releaseLease(runId: string, instanceId: string): Promise<void>
  updateRun(runId: string, from: Run["status"], to: Run["status"], fields?: Partial<Run>): Promise<Run>
  appendItem(item: Omit<TimelineItem, "id" | "timelineIndex" | "createdAt">): Promise<TimelineItem>
  appendEvent(runId: string, type: string, data: Record<string, unknown>): Promise<RunEvent>
  getEvents(ownerUserId: string, runId: string, after: number): Promise<RunEvent[]>
  getTimeline(ownerUserId: string, conversationId: string): Promise<{ conversation: Conversation, turns: Array<{ id: string, turnIndex: number, status: string, input: string, run?: Run, items: TimelineItem[] }> } | undefined>
}
