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
import { Bell, Loader2, Mail, QrCode, Server, Webhook } from 'lucide-react'
import QRCode from 'qrcode'
import { useState, useEffect, useCallback, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { IconWeChat } from '@/assets/brand-icons'
import { PasswordInput } from '@/components/password-input'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { ROLE } from '@/lib/roles'

import { updateUserSettings, getWeChatQRCode, pollWeChatQRStatus } from '../../api'
import {
  DEFAULT_QUOTA_WARNING_THRESHOLD,
  NOTIFICATION_METHODS,
} from '../../constants'
import { parseUserSettings } from '../../lib'
import type { UserProfile, UserSettings, NotifyType } from '../../types'

const NOTIFICATION_ICONS: Record<NotifyType, typeof Mail> = {
  email: Mail,
  webhook: Webhook,
  bark: Bell,
  gotify: Server,
  wechat: IconWeChat as unknown as typeof Mail,
}

const NOTIFICATION_VALUES = new Set<NotifyType>(
  NOTIFICATION_METHODS.map((method) => method.value)
)

function normalizeNotifyType(value: unknown): NotifyType {
  return typeof value === 'string' &&
    NOTIFICATION_VALUES.has(value as NotifyType)
    ? (value as NotifyType)
    : 'email'
}

// ============================================================================
// Settings Tab Component
// ============================================================================

interface NotificationTabProps {
  profile: UserProfile | null
  onUpdate: () => void
}

export function NotificationTab({ profile, onUpdate }: NotificationTabProps) {
  const { t } = useTranslation()
  const isAdmin = (profile?.role ?? 0) >= ROLE.ADMIN
  const [loading, setLoading] = useState(false)
  // WeChat QR binding state
  const [qrLoading, setQrLoading] = useState(false)
  const [qrCodeDataUrl, setQrCodeDataUrl] = useState<string | null>(null)
  const [qrSessionKey, setQrSessionKey] = useState<string | null>(null)
  const [qrStatus, setQrStatus] = useState<string>('') // wait | scaned | confirmed | expired
  const [qrError, setQrError] = useState<string>('')
  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const [settings, setSettings] = useState<UserSettings>({
    notify_type: 'email',
    quota_warning_threshold: DEFAULT_QUOTA_WARNING_THRESHOLD,
    notification_email: '',
    webhook_url: '',
    webhook_secret: '',
    bark_url: '',
    gotify_url: '',
    gotify_token: '',
    gotify_priority: 5,
    accept_unset_model_ratio_model: false,
    record_ip_log: false,
    upstream_model_update_notify_enabled: false,
    wechat_user_id: '',
  })

  // Update form field helper
  const updateField = useCallback(
    <K extends keyof UserSettings>(field: K, value: UserSettings[K]) => {
      setSettings((prev) => ({ ...prev, [field]: value }))
    },
    []
  )

  useEffect(() => {
    if (profile?.setting) {
      const parsed = parseUserSettings(profile.setting)
      setSettings({
        notify_type: normalizeNotifyType(parsed.notify_type),
        quota_warning_threshold:
          parsed.quota_warning_threshold ?? DEFAULT_QUOTA_WARNING_THRESHOLD,
        notification_email: parsed.notification_email ?? '',
        webhook_url: parsed.webhook_url ?? '',
        webhook_secret: parsed.webhook_secret ?? '',
        bark_url: parsed.bark_url ?? '',
        gotify_url: parsed.gotify_url ?? '',
        gotify_token: parsed.gotify_token ?? '',
        gotify_priority: parsed.gotify_priority ?? 5,
        accept_unset_model_ratio_model:
          parsed.accept_unset_model_ratio_model || false,
        record_ip_log: parsed.record_ip_log || false,
        upstream_model_update_notify_enabled:
          parsed.upstream_model_update_notify_enabled || false,
        wechat_user_id: parsed.wechat_user_id ?? '',
      })
    }
  }, [profile])

  // WeChat QR binding handlers
  const startQrBinding = async () => {
    setQrLoading(true)
    setQrError('')
    try {
      const res = await getWeChatQRCode()
      if (res.success && res.qrcode_url && res.session_key) {
        // Generate QR code image from the URL data
        const dataUrl = await QRCode.toDataURL(res.qrcode_url, {
          width: 256,
          margin: 1,
          color: { dark: '#000000', light: '#ffffff' },
        })
        setQrCodeDataUrl(dataUrl)
        setQrSessionKey(res.session_key)
        setQrStatus('wait')
        // Start polling
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

  const startPolling = (sessionKey: string) => {
    stopPolling()
    let active = true
    pollingRef.current = active as unknown as ReturnType<typeof setInterval>
    const poll = async () => {
      while (active) {
        try {
          const res = await pollWeChatQRStatus(sessionKey)
          if (!active) return
          if (res.success && res.data) {
            setQrStatus(res.data.status)
            if (res.data.status === 'confirmed' && res.data.wechat_user_id) {
              active = false
              updateField('wechat_user_id', res.data.wechat_user_id)
              updateField('notify_type', 'wechat')
              toast.success(t('WeChat binding successful!'))
              setTimeout(() => {
                setQrCodeDataUrl(null)
                setQrSessionKey(null)
              }, 2000)
              return
            } else if (res.data.status === 'expired') {
              active = false
              setQrError(t('QR code expired. Please try again.'))
              return
            }
          }
        } catch {
          // network error, retry after brief delay
          if (active) await new Promise(r => setTimeout(r, 1000))
        }
      }
    }
    poll()
  }

  const stopPolling = () => {
    if (pollingRef.current) {
      clearInterval(pollingRef.current)
      pollingRef.current = null
    }
  }

  // Cleanup polling on unmount
  useEffect(() => {
    return () => stopPolling()
  }, [])

  const cancelQrBinding = () => {
    stopPolling()
    setQrCodeDataUrl(null)
    setQrSessionKey(null)
    setQrStatus('')
    setQrError('')
  }

  const handleSave = async () => {
    try {
      setLoading(true)
      const response = await updateUserSettings(settings)

      if (response.success) {
        toast.success(t('Settings updated successfully'))
        onUpdate()
      } else {
        toast.error(response.message || t('Failed to update settings'))
      }
    } catch (_error) {
      toast.error(t('Failed to update settings'))
    } finally {
      setLoading(false)
    }
  }

  const notifyType = normalizeNotifyType(settings.notify_type)

  return (
    <div className='space-y-4 sm:space-y-6'>
      {/* Notification Type */}
      <div className='space-y-2.5'>
        <Label>{t('Notification Method')}</Label>
        <ToggleGroup
          value={[notifyType]}
          onValueChange={(value) => {
            const nextValue = value.find((item) => item !== notifyType)
            if (nextValue)
              updateField('notify_type', normalizeNotifyType(nextValue))
          }}
          aria-label={t('Notification Method')}
          variant='outline'
          size='lg'
          spacing={2}
          className='grid w-full grid-cols-2 gap-2 sm:grid-cols-4 sm:gap-3'
        >
          {NOTIFICATION_METHODS.map((method) => {
            const Icon = NOTIFICATION_ICONS[method.value]
            return (
              <ToggleGroupItem
                key={method.value}
                value={method.value}
                className='h-auto min-h-14 w-full flex-col gap-1.5 px-3 py-3 sm:min-h-16'
              >
                <Icon className='h-4 w-4 sm:h-5 sm:w-5' />
                <span className='max-w-full truncate text-xs font-medium sm:text-sm'>
                  {t(method.label)}
                </span>
              </ToggleGroupItem>
            )
          })}
        </ToggleGroup>
      </div>

      {/* Warning Threshold */}
      <div className='space-y-1.5'>
        <Label htmlFor='threshold'>{t('Quota Warning Threshold')}</Label>
        <Input
          id='threshold'
          type='number'
          className='h-9'
          value={settings.quota_warning_threshold}
          onChange={(e) =>
            updateField('quota_warning_threshold', Number(e.target.value))
          }
          placeholder={t('Enter threshold')}
        />
        <p className='text-muted-foreground text-xs'>
          {t('Get notified when balance falls below this value')}
        </p>
      </div>

      {/* Email Settings */}
      {notifyType === 'email' && (
        <div className='space-y-1.5'>
          <Label htmlFor='notifyEmail'>{t('Notification Email')}</Label>
          <Input
            id='notifyEmail'
            type='email'
            className='h-9'
            value={settings.notification_email}
            onChange={(e) => updateField('notification_email', e.target.value)}
            placeholder={t('Leave empty to use account email')}
          />
        </div>
      )}

      {/* Webhook Settings */}
      {notifyType === 'webhook' && (
        <>
          <div className='space-y-1.5'>
            <Label htmlFor='webhookUrl'>{t('Webhook URL')}</Label>
            <Input
              id='webhookUrl'
              type='url'
              className='h-9'
              value={settings.webhook_url}
              onChange={(e) => updateField('webhook_url', e.target.value)}
              placeholder={t('https://example.com/webhook')}
            />
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='webhookSecret'>{t('Webhook Secret')}</Label>
            <PasswordInput
              id='webhookSecret'
              value={settings.webhook_secret}
              onChange={(e) => updateField('webhook_secret', e.target.value)}
              placeholder={t('Enter secret key')}
            />
          </div>
        </>
      )}

      {/* Bark Settings */}
      {notifyType === 'bark' && (
        <div className='space-y-1.5'>
          <Label htmlFor='barkUrl'>{t('Bark Push URL')}</Label>
          <Input
            id='barkUrl'
            type='url'
            className='h-9'
            value={settings.bark_url}
            onChange={(e) => updateField('bark_url', e.target.value)}
            placeholder={t('https://api.day.app/yourkey/{{title}}/{{content}}')}
          />
          <p className='text-muted-foreground text-xs'>
            {t('Template variables:')} {'{{title}}'}, {'{{content}}'}
          </p>
        </div>
      )}

      {/* Gotify Settings */}
      {notifyType === 'gotify' && (
        <>
          <div className='space-y-1.5'>
            <Label htmlFor='gotifyUrl'>{t('Gotify Server URL')}</Label>
            <Input
              id='gotifyUrl'
              type='url'
              className='h-9'
              value={settings.gotify_url}
              onChange={(e) => updateField('gotify_url', e.target.value)}
              placeholder={t('https://gotify.example.com')}
            />
            <p className='text-muted-foreground text-xs'>
              {t('Enter the full URL of your Gotify server')}
            </p>
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='gotifyToken'>{t('Gotify Application Token')}</Label>
            <PasswordInput
              id='gotifyToken'
              value={settings.gotify_token}
              onChange={(e) => updateField('gotify_token', e.target.value)}
              placeholder={t('Enter application token')}
            />
            <p className='text-muted-foreground text-xs'>
              {t('Token obtained from your Gotify application')}
            </p>
          </div>
          <div className='space-y-1.5'>
            <Label htmlFor='gotifyPriority'>{t('Message Priority')}</Label>
            <Input
              id='gotifyPriority'
              type='number'
              className='h-9'
              min='0'
              max='10'
              value={settings.gotify_priority}
              onChange={(e) =>
                updateField('gotify_priority', Number(e.target.value))
              }
              placeholder='5'
            />
            <p className='text-muted-foreground text-xs'>
              {t(
                'Priority level from 0 (lowest) to 10 (highest), default is 5'
              )}
            </p>
          </div>
          <div className='bg-muted/50 rounded-lg border p-3 sm:p-4'>
            <h5 className='mb-1.5 text-sm font-medium sm:mb-2'>
              {t('Setup Instructions')}
            </h5>
            <ol className='text-muted-foreground space-y-1 text-xs'>
              <li>{t('1. Create an application in your Gotify server')}</li>
              <li>{t('2. Copy the application token')}</li>
              <li>{t('3. Enter your Gotify server URL and token above')}</li>
            </ol>
            <p className='text-muted-foreground mt-3 text-xs'>
              {t('Learn more:')}{' '}
              <a
                href='https://gotify.net/'
                target='_blank'
                rel='noopener noreferrer'
                className='text-primary underline underline-offset-4'
              >
                {t('Gotify Documentation')}
              </a>
            </p>
          </div>
        </>
      )}

      {/* WeChat Bot Settings */}
      {notifyType === 'wechat' && (
        <>
          {settings.wechat_user_id ? (
            <div className='rounded-lg border border-green-200 bg-green-50 p-4 dark:border-green-800 dark:bg-green-950'>
              <div className='flex items-center gap-2'>
                <IconWeChat className='h-5 w-5 text-green-600' />
                <span className='font-medium text-green-700 dark:text-green-400'>
                  {t('WeChat Bound')}
                </span>
              </div>
              <p className='text-muted-foreground mt-2 text-xs'>
                {t('WeChat User ID')}: {settings.wechat_user_id}
              </p>
              <Button
                variant='outline'
                size='sm'
                className='mt-3'
                onClick={() => updateField('wechat_user_id', '')}
              >
                {t('Unbind')}
              </Button>
            </div>
          ) : qrCodeDataUrl ? (
            <div className='rounded-lg border p-4'>
              <div className='flex flex-col items-center gap-3'>
                <img
                  src={qrCodeDataUrl}
                  alt={t('WeChat binding QR code')}
                  className='size-48 rounded-lg border object-contain'
                />
                {qrStatus === 'wait' && (
                  <p className='text-muted-foreground text-sm'>
                    <Loader2 className='mr-1.5 inline-block h-3.5 w-3.5 animate-spin' />
                    {t('Waiting for scan...')}
                  </p>
                )}
                {qrStatus === 'scaned' && (
                  <p className='text-primary text-sm font-medium'>
                    {t('Scanned! Confirm on your phone...')}
                  </p>
                )}
                {qrStatus === 'confirmed' && (
                  <p className='text-green-600 text-sm font-medium'>
                    {t('Binding successful!')}
                  </p>
                )}
                {qrError && (
                  <p className='text-destructive text-sm'>{qrError}</p>
                )}
                <Button variant='ghost' size='sm' onClick={cancelQrBinding}>
                  {t('Cancel')}
                </Button>
              </div>
            </div>
          ) : (
            <div className='rounded-lg border p-4'>
              <div className='flex flex-col items-center gap-3 text-center'>
                <QrCode className='text-muted-foreground h-12 w-12' />
                <div>
                  <p className='font-medium'>{t('Bind WeChat for Notifications')}</p>
                  <p className='text-muted-foreground mt-1 text-xs'>
                    {t('Scan a QR code with WeChat to bind your account and receive alert notifications.')}
                  </p>
                </div>
                <Button onClick={startQrBinding} disabled={qrLoading}>
                  {qrLoading && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
                  {qrLoading ? t('Generating QR...') : t('Generate QR Code')}
                </Button>
                {qrError && (
                  <p className='text-destructive text-sm'>{qrError}</p>
                )}
              </div>
            </div>
          )}
        </>
      )}

      {/* Divider */}
      <div className='border-t' />

      {/* Preferences Section */}
      <div className='space-y-3'>
        <div>
          <h4 className='text-sm font-medium'>{t('Preferences')}</h4>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t('Configure your account behavior preferences')}
          </p>
        </div>

        {/* Receive Upstream Model Update Notifications (admin only) */}
        {isAdmin && (
          <div className='flex items-start justify-between gap-3 rounded-lg border p-3 sm:items-center sm:p-4'>
            <div className='space-y-0.5'>
              <Label htmlFor='upstreamModelUpdateNotify'>
                {t('Receive Upstream Model Update Notifications')}
              </Label>
              <p className='text-muted-foreground line-clamp-3 text-xs sm:line-clamp-none sm:text-sm'>
                {t(
                  'Only available for admins. When enabled, you will receive a summary notification via your selected method when the scheduled model check detects upstream model changes or check failures.'
                )}
              </p>
            </div>
            <Switch
              id='upstreamModelUpdateNotify'
              className='shrink-0'
              checked={settings.upstream_model_update_notify_enabled}
              onCheckedChange={(checked) =>
                updateField('upstream_model_update_notify_enabled', checked)
              }
            />
          </div>
        )}

        {/* Accept Unset Model Price */}
        <div className='flex items-start justify-between gap-3 rounded-lg border p-3 sm:items-center sm:p-4'>
          <div className='space-y-0.5'>
            <Label htmlFor='acceptUnsetPrice'>
              {t('Accept Unpriced Models')}
            </Label>
            <p className='text-muted-foreground text-xs sm:text-sm'>
              {t('Allow using models without price configuration')}
            </p>
          </div>
          <Switch
            id='acceptUnsetPrice'
            className='shrink-0'
            checked={settings.accept_unset_model_ratio_model}
            onCheckedChange={(checked) =>
              updateField('accept_unset_model_ratio_model', checked)
            }
          />
        </div>

        {/* Record IP Log */}
        <div className='flex items-start justify-between gap-3 rounded-lg border p-3 sm:items-center sm:p-4'>
          <div className='space-y-0.5'>
            <Label htmlFor='recordIp'>{t('Record IP Address')}</Label>
            <p className='text-muted-foreground text-xs sm:text-sm'>
              {t('Log IP address for usage and error logs')}
            </p>
          </div>
          <Switch
            id='recordIp'
            className='shrink-0'
            checked={settings.record_ip_log}
            onCheckedChange={(checked) => updateField('record_ip_log', checked)}
          />
        </div>
      </div>

      {/* Save Button */}
      <div className='flex justify-end'>
        <Button onClick={handleSave} disabled={loading}>
          {loading && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
          {loading ? t('Saving...') : t('Save Settings')}
        </Button>
      </div>
    </div>
  )
}
