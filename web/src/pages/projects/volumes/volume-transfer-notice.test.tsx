import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { describe, expect, it } from 'vitest'
import { VolumeTransferNotice } from './volume-transfer-notice'

describe('volume transfer notice', () => {
  it('explains why transfer is unavailable when the deployment has not configured it', () => {
    render(<VolumeTransferNotice direction="export" transferEnabled={false} />)

    expect(screen.getByRole('alert')).toHaveTextContent(i18next.t('projectVolumes.transferUnavailableTitle'))
    expect(screen.getByRole('alert')).toHaveTextContent('VOLUME_TRANSFER_JOB_IMAGE')
  })

  it('keeps the direct transfer hint when transfer is configured', () => {
    render(<VolumeTransferNotice direction="import" transferEnabled />)

    expect(screen.getByRole('alert')).toHaveTextContent(i18next.t('projectVolumes.importNoResume'))
    expect(screen.queryByText(i18next.t('projectVolumes.transferUnavailableTitle'))).not.toBeInTheDocument()
  })

  it('maps the unavailable error code to a specific message', () => {
    expect(i18next.exists('errors.volume_transfer.unavailable')).toBe(true)
  })
})
