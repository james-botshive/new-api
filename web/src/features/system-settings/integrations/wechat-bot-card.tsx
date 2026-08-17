/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQueryClient } from '@tanstack/react-query'
import { Loader2, QrCode } from 'lucide-react'
import QRCode from 'qrcode'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { IconWeChat } from '@/assets/brand-icons'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'

import { getWeChatBotQRCode, pollWeChatBotQRStatus } from '../api'

type WeChatBotCardProps = {
  token: string
  botId: string
}

/**
 * WeChat Bot registration card for the monitoring settings section.
 *
 * Without a configured token the button starts an iLink registration: scan
 * the QR code with WeChat and the server stores the returned bot_token,
 * baseurl and bot id. With a token configured the same flow re-scans to
 * refresh the credentials (the bot id changes on every scan).
 */
export function WeChatBotCard({ token, botId }: WeChatBotCardProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [qrLoading, setQrLoading] = useState(false)
  const [qrCodeDataUrl, setQrCodeDataUrl] = useState<string | null>(null)
  const [qrStatus, setQrStatus] = useState('')
  const [qrError, setQrError] = useState('')
  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const registered = Boolean(token)

  const stopPolling = () => {
    if (pollingRef.current) {
      clearInterval(pollingRef.current)
      pollingRef.current = null
    }
  }

  const resetQrState = () => {
    stopPolling()
    setQrCodeDataUrl(null)
    setQrStatus('')
    setQrError('')
    setQrLoading(false)
  }

  const startPolling = (sessionKey: string) => {
    stopPolling()
    let active = true
    pollingRef.current = active as unknown as ReturnType<typeof setInterval>
    const poll = async () => {
      while (active) {
        try {
          const res = await pollWeChatBotQRStatus(sessionKey)
          if (!active) return
          if (res.success && res.data) {
            setQrStatus(res.data.status)
            if (res.data.status === 'confirmed' && res.data.wechat_user_id) {
              active = false
              if (res.data.error) {
                setQrError(
                  t('WeChat Bot registration failed. Please try again.')
                )
                return
              }
              toast.success(t('WeChat Bot registration successful!'))
              void queryClient.invalidateQueries({
                queryKey: ['system-options'],
              })
              setTimeout(() => {
                setDialogOpen(false)
                resetQrState()
              }, 1500)
              return
            } else if (res.data.status === 'expired') {
              active = false
              setQrError(t('QR code expired. Please try again.'))
              return
            }
          }
        } catch {
          // Network error; retry after a brief delay.
          if (active) await new Promise(r => setTimeout(r, 1000))
        }
        if (active) await new Promise(r => setTimeout(r, 1500))
      }
    }
    poll()
  }

  const startQrBinding = async () => {
    setQrLoading(true)
    setQrError('')
    try {
      const res = await getWeChatBotQRCode()
      if (res.success && res.qrcode_url && res.session_key) {
        const dataUrl = await QRCode.toDataURL(res.qrcode_url, {
          width: 256,
          margin: 1,
          color: { dark: '#000000', light: '#ffffff' },
        })
        setQrCodeDataUrl(dataUrl)
        setQrStatus('wait')
        startPolling(res.session_key)
      } else {
        setQrError(res.message || t('Failed to generate QR code'))
      }
    } catch {
      setQrError(t('Failed to connect to WeChat service'))
    } finally {
      setQrLoading(false)
    }
  }

  // Cleanup polling on unmount.
  useEffect(() => {
    return () => stopPolling()
  }, [])

  const statusText = qrStatus === 'scaned' ? t('Scanned! Confirm on your phone...') : t('Waiting for scan...')

  return (
    <div>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div>
          <h4 className='flex items-center gap-2 font-medium'>
            <IconWeChat className='size-4' />
            {t('WeChat Bot')}
          </h4>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t(
              'Scan a QR code with WeChat to register the alert bot and receive notifications.'
            )}
          </p>
        </div>
        <Button
          type='button'
          variant='outline'
          onClick={() => {
            setDialogOpen(true)
            void startQrBinding()
          }}
        >
          <QrCode className='size-4' />
          {registered ? t('Re-scan QR Code') : t('Scan to Register')}
        </Button>
      </div>

      <div className='mt-3 flex items-center gap-2 text-sm'>
        <span
          className={`inline-block size-2 rounded-full ${registered ? 'bg-green-500' : 'bg-muted-foreground/40'}`}
        />
        {registered ? t('Registered') : t('Not registered')}
        {registered && botId ? (
          <span className='text-muted-foreground text-xs'>
            {t('Bot ID')}: {botId}
          </span>
        ) : null}
      </div>

      <Dialog
        open={dialogOpen}
        onOpenChange={(open) => {
          setDialogOpen(open)
          if (!open) resetQrState()
        }}
        title={t('WeChat Bot')}
        description={t(
          'Scan a QR code with WeChat to register the alert bot and receive notifications.'
        )}
        footer={
          <Button
            type='button'
            variant='outline'
            onClick={() => {
              setDialogOpen(false)
              resetQrState()
            }}
          >
            {t('Cancel')}
          </Button>
        }
      >
        <div className='flex flex-col items-center gap-3 py-2'>
          {qrLoading ? (
            <Loader2 className='size-8 animate-spin text-muted-foreground' />
          ) : null}
          {qrCodeDataUrl ? (
            <img
              src={qrCodeDataUrl}
              alt='WeChat QR code'
              width={256}
              height={256}
              className='size-48 rounded-lg border object-contain'
            />
          ) : null}
          {!qrLoading && !qrCodeDataUrl && !qrError ? (
            <p className='text-muted-foreground text-sm'>
              {t('Generating QR...')}
            </p>
          ) : null}
          {qrCodeDataUrl && !qrError ? (
            <p className='text-muted-foreground text-sm'>{statusText}</p>
          ) : null}
          {qrError ? (
            <>
              <p className='text-destructive text-sm'>{qrError}</p>
              <Button type='button' variant='outline' onClick={() => void startQrBinding()}>
                {t('Generate QR Code')}
              </Button>
            </>
          ) : null}
        </div>
      </Dialog>
    </div>
  )
}
