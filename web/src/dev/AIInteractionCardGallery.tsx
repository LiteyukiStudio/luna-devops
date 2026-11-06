import { extremeInteractionCardFixture, interactionCardTemplateFixtures, templateSelectionInteractionCardFixture } from '@/components/common/ai-assistant/interaction-card-fixtures'
import { AIInteractionCards } from '@/components/common/ai-assistant/interaction-cards'
import { AIAssistantTimeline } from '@/components/common/ai-assistant/timeline'

export function AIInteractionCardGallery() {
  const allFixtures = [...Object.values(interactionCardTemplateFixtures), templateSelectionInteractionCardFixture, extremeInteractionCardFixture]
  const requestedFixture = new URLSearchParams(window.location.search).get('fixture')
  if (requestedFixture === 'messages')
    return <MessageActionsFixture />
  const fixtures = requestedFixture
    ? allFixtures.filter(fixture => fixture.generationId === requestedFixture)
    : allFixtures
  return (
    <main className="min-h-screen bg-background p-4">
      <div className="grid items-start gap-4 xl:grid-cols-3">
        {fixtures.map(fixture => (
          <section key={fixture.generationId} className="mx-auto w-full min-w-0 max-w-[420px] overflow-x-hidden rounded-container border border-border bg-surface p-4 text-[13px] shadow-sm" data-ai-gallery-template={fixture.template}>
            <AIInteractionCards arguments={fixture} onAction={async () => true} />
          </section>
        ))}
      </div>
    </main>
  )
}

function MessageActionsFixture() {
  return (
    <main className="grid min-h-screen place-items-center bg-background p-4">
      <section className="flex h-[520px] w-full max-w-[420px] overflow-hidden rounded-container border border-border bg-surface text-[13px] shadow-overlay">
        <AIAssistantTimeline
          blocks={[
            { id: 'user', turnId: 'turn', index: -1, type: 'message', role: 'user', status: 'completed', text: '请帮我检查这个应用最近一次部署。', createdAt: '2026-08-01T17:02:00+08:00' },
            { id: 'assistant', turnId: 'turn', index: 0, type: 'message', role: 'assistant', status: 'completed', text: '部署已经完成，当前副本运行正常。\n\n- 可用副本：2/2\n- 最近发布：17:03', createdAt: '2026-08-01T17:03:00+08:00' },
          ]}
          error={null}
          generating={false}
          loading={false}
          onAction={async () => true}
          onApproval={async () => {}}
          onResend={() => {}}
          onRetry={() => {}}
        />
      </section>
    </main>
  )
}
