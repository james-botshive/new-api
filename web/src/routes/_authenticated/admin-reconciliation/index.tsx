import { createFileRoute } from '@tanstack/react-router'
import { ReconciliationTable } from '@/features/usage-logs/components/reconciliation-table'

export const Route = createFileRoute('/_authenticated/admin-reconciliation/')({
  component: () => (
    <div className='p-4 sm:p-6'>
      <h1 className='mb-4 text-2xl font-bold'>Reconciliation</h1>
      <ReconciliationTable />
    </div>
  ),
})
