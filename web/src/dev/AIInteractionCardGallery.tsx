import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { extremeInteractionCardFixture, interactionCardTemplateFixtures } from '@/components/common/ai-assistant/interaction-card-fixtures'
import { AIInteractionCards } from '@/components/common/ai-assistant/interaction-cards'
import { AIAssistantLauncher } from '@/components/common/ai-assistant/launcher'
import { Button } from '@/components/ui/button'

export function AIInteractionCardGallery() {
  const allFixtures = [...Object.values(interactionCardTemplateFixtures), extremeInteractionCardFixture]
  const requestedFixture = new URLSearchParams(window.location.search).get('fixture')
  if (requestedFixture === 'launcher')
    return <LauncherFixture />
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
