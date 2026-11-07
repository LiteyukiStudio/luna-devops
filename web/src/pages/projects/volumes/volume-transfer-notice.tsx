import { CircleAlert, Download, Upload } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'

/** 导入导出对话框顶部提示；部署未配置数据卷直传时改为说明原因，提交入口同时禁用。 */
export function VolumeTransferNotice({ direction, transferEnabled }: { direction: 'export' | 'import', transferEnabled: boolean }) {
  const { t } = useTranslation()
  if (!transferEnabled) {
    return (
      <Alert variant="warning">
        <CircleAlert />
        <AlertTitle>{t('projectVolumes.transferUnavailableTitle')}</AlertTitle>
        <AlertDescription>{t('projectVolumes.transferUnavailableDescription')}</AlertDescription>
      </Alert>
    )
  }
  return (
    <Alert>
      {direction === 'import' ? <Upload /> : <Download />}
      <AlertTitle>{t('projectVolumes.directTransfer')}</AlertTitle>
      <AlertDescription>{t(direction === 'import' ? 'projectVolumes.importNoResume' : 'projectVolumes.exportNoResume')}</AlertDescription>
    </Alert>
  )
}
