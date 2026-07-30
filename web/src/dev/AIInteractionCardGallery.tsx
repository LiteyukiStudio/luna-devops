import { Sparkles } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { extremeInteractionCardFixture, interactionCardTemplateFixtures } from '@/components/common/ai-assistant/interaction-card-fixtures'
import { AIInteractionCards } from '@/components/common/ai-assistant/interaction-cards'
import { aiAssistantLauncherClassName } from '@/components/common/ai-assistant/launcher'
import { Button } from '@/components/ui/button'

export function AIInteractionCardGallery() {
  const { t } = useTranslation()
  const allFixtures = [...Object.values(interactionCardTemplateFixtures), extremeInteractionCardFixture]
  const requestedFixture = new URLSearchParams(window.location.search).get('fixture')
  if (requestedFixture === 'launcher') {
    return (
      <main className="grid min-h-screen place-items-center bg-primary-subtle">
        <Button aria-label={t('aiAssistant.open')} className={aiAssistantLauncherClassName} size="icon">
          <Sparkles className="size-5" />
        </Button>
      </main>
    )
  }
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
