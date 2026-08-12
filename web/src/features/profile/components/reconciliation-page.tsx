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

import { getPersonalReconciliation } from '../api'
import type { ReconItem, ReconResult } from '../api'

function formatCost(quota: number): string {
  return `$${(quota / 500000).toFixed(4)}`
}

function fmt(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

function todayStart(): string {
  return new Date().toISOString().slice(0, 10)
}

function daysAgo(n: number): string {
  const d = new Date()
  d.setDate(d.getDate() - n)
  return d.toISOString().slice(0, 10)
}

export function ReconciliationPage() {
  const { t } = useTranslation()
  const [loading, setLoading] = useState(false)
  const [data, setData] = useState<ReconResult | null>(null)
  const [error, setError] = useState('')
  const [startDate, setStartDate] = useState(daysAgo(7))
  const [endDate, setEndDate] = useState(todayStart())
  const [modelFilter, setModelFilter] = useState('')
  const [modelOptions, setModelOptions] = useState<{ value: string; label: string }[]>([])

  // Fetch distinct models the user has used
  useEffect(() => {
    api.get('/api/log/self/models').then((res: any) => {
      const models: string[] = res.data?.data ?? []
      setModelOptions(models.map((m) => ({ value: m, label: m })))
    }).catch(() => {})
  }, [])

  const query = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const startTs = startDate ? Math.floor(new Date(startDate).getTime() / 1000) : 0
      const endTs = endDate ? Math.floor(new Date(endDate + 'T23:59:59').getTime() / 1000) : 0
      const res = await getPersonalReconciliation({
        start_timestamp: startTs || undefined,
        end_timestamp: endTs || undefined,
        model_name: modelFilter.trim() || undefined,
      })
      if (res.success && res.data) {
        setData(res.data)
      } else {
        setError(res.message || t('Query failed'))
      }
    } catch {
      setError(t('Network error'))
    } finally {
      setLoading(false)
    }
  }, [startDate, endDate, modelFilter, t])

  const items = data?.items || []
  const total = data?.total

  return (
    <div className='mx-auto max-w-5xl space-y-6 p-4 sm:p-6'>
      <div>
        <h1 className='text-2xl font-bold'>{t('Reconciliation')}</h1>
        <p className='text-muted-foreground mt-1 text-sm'>{t('View your upstream API usage grouped by model')}</p>
      </div>

      {/* Filters */}
      <Card>
        <CardContent className='flex flex-wrap items-end gap-3 pt-6'>
          <div className='space-y-1'>
            <Label className='text-xs'>{t('Start Date')}</Label>
            <Input type='date' className='h-9 w-36' value={startDate} onChange={(e) => setStartDate(e.target.value)} />
          </div>
          <div className='space-y-1'>
            <Label className='text-xs'>{t('End Date')}</Label>
            <Input type='date' className='h-9 w-36' value={endDate} onChange={(e) => setEndDate(e.target.value)} />
          </div>
          <div className='space-y-1'>
            <Label className='text-xs'>{t('Model')}</Label>
            <ComboboxInput
              options={modelOptions}
              value={modelFilter}
              onValueChange={setModelFilter}
              placeholder={t('All models')}
              emptyText={t('No models found')}
              allowCustomValue
              className='w-52'
            />
          </div>
          <Button onClick={query} disabled={loading}>
            {loading ? <Loader2 className='mr-1 h-4 w-4 animate-spin' /> : <Search className='mr-1 h-4 w-4' />}
            {t('Query')}
          </Button>
        </CardContent>
      </Card>

      {error && <p className='text-destructive text-sm'>{error}</p>}

      {/* Summary cards */}
      {total && total.count > 0 && (
        <div className='grid grid-cols-2 gap-3 sm:grid-cols-5'>
          <StatCard label={t('Total Calls')} value={String(total.count)} />
          <StatCard label={t('Input Tokens')} value={fmt(total.prompt_tokens)} />
          <StatCard label={t('Output Tokens')} value={fmt(total.completion_tokens)} />
          <StatCard label={t('Cache Hit')} value={fmt(total.cache_hit_tokens)} />
          <StatCard label={t('Est. Cost')} value={formatCost(total.quota)} />
        </div>
      )}

      {/* Table */}
      {items.length > 0 && (
        <Card>
          <CardHeader><CardTitle className='text-base'>{t('Details')}</CardTitle></CardHeader>
          <div className='overflow-x-auto'>
            <table className='w-full text-xs'>
              <thead className='bg-muted/50 border-b'>
                <tr>
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
                    <Td className='font-medium'>{item.model_name}</Td>
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
        </Card>
      )}

      {!loading && items.length === 0 && (
        <p className='text-muted-foreground py-12 text-center text-sm'>{t('No records found')}</p>
      )}
    </div>
  )
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <Card>
      <CardContent className='pt-6'>
        <p className='text-muted-foreground text-xs'>{label}</p>
        <p className='mt-1 text-lg font-semibold'>{value}</p>
      </CardContent>
    </Card>
  )
}

function Th({ children }: { children: React.ReactNode }) {
  return <th className='px-3 py-2 text-left font-medium text-muted-foreground'>{children}</th>
}

function Td({ className, children }: { className?: string; children: React.ReactNode }) {
  return <td className={`px-3 py-1.5 ${className || ''}`}>{children}</td>
}
