import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { extremeInteractionCardFixture, interactionCardTemplateFixtures, templateSelectionInteractionCardFixture } from '@/components/common/ai-assistant/interaction-card-fixtures'
import { AIInteractionCards } from '@/components/common/ai-assistant/interaction-cards'
import { AIAssistantLauncher } from '@/components/common/ai-assistant/launcher'
import { AIAssistantTimeline } from '@/components/common/ai-assistant/timeline'
import { Button } from '@/components/ui/button'

export function AIInteractionCardGallery() {
  const allFixtures = [...Object.values(interactionCardTemplateFixtures), templateSelectionInteractionCardFixture, extremeInteractionCardFixture]
  const requestedFixture = new URLSearchParams(window.location.search).get('fixture')
  if (requestedFixture === 'launcher')
    return <LauncherFixture />
  if (requestedFixture === 'messages')
    return <MessageActionsFixture />
  const fixtures = requestedFixture
    ? allFixtures.filter(fixture => fixture.generationId === requestedFixture)
    : allFixtures
  return (
    <main className="min-h-screen bg-primary-subtle p-4">
      <div className="grid items-start gap-4 xl:grid-cols-3">
        {fixtures.map(fixture => (
          <section key={fixture.generationId} className="mx-auto w-full min-w-0 max-w-[420px] overflow-x-hidden rounded-feature bg-surface-inset p-3 text-[13px]" data-ai-gallery-template={fixture.template}>
            <AIInteractionCards arguments={fixture} onAction={async () => true} />
          </section>
        ))}
      </div>
    </main>
  )
}

function MessageActionsFixture() {
  return (
    <main className="grid min-h-screen place-items-center bg-primary-subtle p-4">
      <section className="flex h-[520px] w-full max-w-[420px] overflow-hidden rounded-feature bg-surface shadow-overlay">
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

function LauncherFixture() {
  const { t } = useTranslation()
  const [opened, setOpened] = useState(false)
  const [position, setPosition] = useState({ x: 24, y: 24 })

  return (
    <main className="grid min-h-screen place-items-center bg-primary-subtle">
      {!opened && (
        <AIAssistantLauncher
          label={t('aiAssistant.open')}
          position={position}
          onOpen={() => setOpened(true)}
          onPositionChange={setPosition}
        />
      )}
      {opened && (
        <section aria-label={t('aiAssistant.title')} className="grid gap-4 rounded-feature bg-surface p-6 shadow-overlay">
          <p>{t('aiAssistant.title')}</p>
          <Button onClick={() => setOpened(false)}>{t('common.close')}</Button>
        </section>
      )}
    </main>
  )
}
