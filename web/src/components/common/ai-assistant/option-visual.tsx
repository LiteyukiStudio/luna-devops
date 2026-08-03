import type { AIOptionIconName, AIOptionVisual } from '@luna-devops/ai-interaction-card-contract'
import type { LucideIcon } from 'lucide-react'
import {
  Activity,
  BookOpen,
  Box,
  CircleHelp,
  Cloud,
  Code2,
  Database,
  FolderKanban,
  Gauge,
  GitBranch,
  Globe2,
  ImageIcon,
  ListChecks,
  MessageCircle,
  Package,
  Rocket,
  Search,
  Settings,
  ShieldCheck,
  Terminal,
  Wrench,
} from 'lucide-react'
import { useState } from 'react'

const optionIcons: Record<AIOptionIconName, LucideIcon> = {
  'activity': Activity,
  'book-open': BookOpen,
  'box': Box,
  'circle-help': CircleHelp,
  'cloud': Cloud,
  'code': Code2,
  'database': Database,
  'folder-kanban': FolderKanban,
  'gauge': Gauge,
  'git-branch': GitBranch,
  'globe': Globe2,
  'list-checks': ListChecks,
  'message-circle': MessageCircle,
  'package': Package,
  'rocket': Rocket,
  'search': Search,
  'settings': Settings,
  'shield-check': ShieldCheck,
  'terminal': Terminal,
  'wrench': Wrench,
}

export function AIOptionLeadingVisual({ visual }: { visual: AIOptionVisual }) {
  if (visual.type === 'emoji') {
    return <span aria-hidden="true" className="text-[13px] leading-none">{visual.value}</span>
  }
  if (visual.type === 'icon') {
    const Icon = optionIcons[visual.value]
    return <Icon aria-hidden="true" className="size-3.5" />
  }
  return <AIOptionImage src={visual.value} />
}

function AIOptionImage({ src }: { src: string }) {
  const [failed, setFailed] = useState(false)
  if (failed)
    return <ImageIcon aria-hidden="true" className="size-3.5" />
  return (
    <img
      alt=""
      className="size-4 rounded-full object-cover"
      draggable={false}
      referrerPolicy="no-referrer"
      src={src}
      onError={() => setFailed(true)}
    />
  )
}
