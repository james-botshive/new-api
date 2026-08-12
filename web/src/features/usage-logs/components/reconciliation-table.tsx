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
import { Loader2, Search } from 'lucide-react'
import { useState, useCallback, useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ComboboxInput } from '@/components/ui/combobox-input'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'

import { getAdminReconciliation } from '../api'
import type { ReconItem, ReconResult } from '../api'

function formatCost(quota: number): string {
  return `$${(quota / 500000).toFixed(4)}`
}

function fmt(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

function fmtDatetime(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

function defaultStart(): string {
  const d = new Date()
  d.setDate(d.getDate() - 7)
  d.setHours(0, 0, 0, 0)
  return fmtDatetime(d)
}

function defaultEnd(): string {
  const d = new Date()
  d.setHours(23, 59, 59, 0)
  return fmtDatetime(d)
}

export function ReconciliationTable() {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [data, setData] = useState<ReconResult | null>(null)
  const [error, setError] = useState('')
  const [startTime, setStartTime] = useState(defaultStart())
  const [endTime, setEndTime] = useState(defaultEnd())
  const [modelFilter, setModelFilter] = useState('')
  const [userFilter, setUserFilter] = useState('')
  const [channelFilter, setChannelFilter] = useState('')
  const [groupFilter, setGroupFilter] = useState('')

  const [userOptions, setUserOptions] = useState<{ value: string; label: string }[]>([])
  const [modelOptions, setModelOptions] = useState<{ value: string; label: string }[]>([])
  const [channelOptions] = useState<{ value: string; label: string }[]>([])
  const [groupOptions] = useState<{ value: string; label: string }[]>([])

  // Fetch filter options
  useEffect(() => {
    api.get('/api/log/reconciliation/filters').then((res: any) => {
      const data = res.data?.data ?? {}
      const users: string[] = data.users ?? []
      const models: string[] = data.models ?? []
      setUserOptions(users.map((u) => ({ value: u, label: u })))
      setModelOptions(models.map((m) => ({ value: m, label: m })))
    }).catch(() => {})
  }, [])

  const query = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const startTs = startTime ? Math.floor(new Date(startTime).getTime() / 1000) : 0
      const endTs = endTime ? Math.floor(new Date(endTime).getTime() / 1000) : 0
      const res = await getAdminReconciliation({
        start_timestamp: startTs || undefined,
        end_timestamp: endTs || undefined,
        model_name: modelFilter.trim() || undefined,
        username: userFilter.trim() || undefined,
        channel: channelFilter || undefined,
        group: groupFilter.trim() || undefined,
      } as any)
      if (res.success) {
        setData(res.data)
      } else {
        setError(t('Query failed'))
      }
    } catch {
      setError(t('Network error'))
    } finally {
      setLoading(false)
    }
  }, [startTime, endTime, modelFilter, userFilter, channelFilter, groupFilter, t])

  const items = data?.items || []
  const total = data?.total

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-end gap-3'>
        <div className='space-y-1'>
          <Label className='text-xs'>{t('Start')}</Label>
          <Input type='datetime-local' step='1' className='h-9 w-52' value={startTime} onChange={(e) => setStartTime(e.target.value)} />
        </div>
        <div className='space-y-1'>
          <Label className='text-xs'>{t('End')}</Label>
          <Input type='datetime-local' step='1' className='h-9 w-52' value={endTime} onChange={(e) => setEndTime(e.target.value)} />
        </div>
        <div className='space-y-1'>
          <Label className='text-xs'>{t('Model')}</Label>
          <ComboboxInput options={modelOptions} value={modelFilter} onValueChange={setModelFilter} placeholder={t('All models')} emptyText={t('No models found')} allowCustomValue className='w-48' />
        </div>
        <div className='space-y-1'>
          <Label className='text-xs'>{t('User')}</Label>
          <ComboboxInput options={userOptions} value={userFilter} onValueChange={setUserFilter} placeholder={t('All users')} emptyText={t('No users found')} allowCustomValue className='w-36' />
        </div>
        <div className='space-y-1'>
          <Label className='text-xs'>{t('Channel')}</Label>
          <ComboboxInput options={channelOptions} value={channelFilter} onValueChange={setChannelFilter} placeholder={t('All')} emptyText='' allowCustomValue className='w-28' />
        </div>
        <div className='space-y-1'>
          <Label className='text-xs'>{t('Group')}</Label>
          <ComboboxInput options={groupOptions} value={groupFilter} onValueChange={setGroupFilter} placeholder={t('All')} emptyText='' allowCustomValue className='w-28' />
        </div>
        <Button size='sm' onClick={query} disabled={loading}>
          {loading ? <Loader2 className='mr-1 h-4 w-4 animate-spin' /> : <Search className='mr-1 h-4 w-4' />}
          {t('Query')}
        </Button>
      </div>

      {error && <p className='text-destructive text-sm'>{error}</p>}

      {total && total.count > 0 && (
        <div className='grid grid-cols-2 gap-3 sm:grid-cols-5'>
          <StatCard label={t('Total Calls')} value={String(total.count)} />
          <StatCard label={t('Input Tokens')} value={fmt(total.prompt_tokens)} />
          <StatCard label={t('Output Tokens')} value={fmt(total.completion_tokens)} />
          <StatCard label={t('Cache Hit')} value={fmt(total.cache_hit_tokens)} />
          <StatCard label={t('Est. Cost')} value={formatCost(total.quota)} />
        </div>
      )}

      {items.length > 0 && (
        <div className='overflow-x-auto rounded-lg border'>
          <table className='w-full text-xs'>
            <thead className='bg-muted/50 border-b'>
              <tr>
                <Th>{t('User')}</Th>
                <Th>{t('Model')}</Th>
                <Th>{t('Calls')}</Th>
                <Th>{t('Input')}</Th>
                <Th>{t('Output')}</Th>
                <Th>{t('Cache Hit')}</Th>
                <Th>{t('Cache W5m')}</Th>
                <Th>{t('Cache W1h')}</Th>
                <Th>{t('Cost')}</Th>
              </tr>
            </thead>
            <tbody>
              {items.map((item, i) => (
                <tr key={i} className='border-b last:border-0'>
                  <Td className='font-medium'>{item.username || '-'}</Td>
                  <Td>{item.model_name}</Td>
                  <Td>{item.count}</Td>
                  <Td>{fmt(item.prompt_tokens)}</Td>
                  <Td>{fmt(item.completion_tokens)}</Td>
                  <Td>{fmt(item.cache_hit_tokens)}</Td>
                  <Td>{fmt(item.cache_write_5m_tokens)}</Td>
                  <Td>{fmt(item.cache_write_1h_tokens)}</Td>
                  <Td className='font-medium'>{formatCost(item.quota)}</Td>
                </tr>
              ))}
            </tbody>
            <tfoot className='bg-muted/30 border-t font-medium'>
              <tr>
                <Th>{t('Total')}</Th>
                <Th>-</Th>
                <Th>{total?.count}</Th>
                <Th>{fmt(total?.prompt_tokens || 0)}</Th>
                <Th>{fmt(total?.completion_tokens || 0)}</Th>
                <Th>{fmt(total?.cache_hit_tokens || 0)}</Th>
                <Th>{fmt(total?.cache_write_5m_tokens || 0)}</Th>
                <Th>{fmt(total?.cache_write_1h_tokens || 0)}</Th>
                <Th>{formatCost(total?.quota || 0)}</Th>
              </tr>
            </tfoot>
          </table>
        </div>
      )}

      {!loading && items.length === 0 && (
        <p className='text-muted-foreground py-8 text-center text-sm'>{t('No records found')}</p>
      )}
    </div>
  )
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className='rounded-lg border p-3'>
      <p className='text-muted-foreground text-xs'>{label}</p>
      <p className='mt-1 text-lg font-semibold'>{value}</p>
    </div>
  )
}

function Th({ children }: { children: React.ReactNode }) {
  return <th className='px-3 py-2 text-left font-medium text-muted-foreground'>{children}</th>
}

function Td({ className, children }: { className?: string; children: React.ReactNode }) {
  return <td className={`px-3 py-1.5 ${className || ''}`}>{children}</td>
}
