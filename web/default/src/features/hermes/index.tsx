import { useEffect, useState } from 'react'
import { ExternalLink, RefreshCw, RadioTower, Link2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { SectionPageLayout } from '@/components/layout'
import { getHermesTenantSelf, getLatestHermesPairingSession } from './api'
import type { HermesPairingSession, HermesTenant } from './types'

function StatusBadge({ value }: { value?: string }) {
  const normalized = value || 'unknown'
  const healthy = ['active', 'ready', 'paired', 'completed'].includes(
    normalized
  )
  return (
    <Badge variant={healthy ? 'default' : 'secondary'} className='capitalize'>
      {normalized}
    </Badge>
  )
}

function DetailRow(props: { label: string; value?: string | number | null }) {
  return (
    <div className='flex flex-col gap-1 rounded-lg border px-3 py-2.5 sm:flex-row sm:items-center sm:justify-between'>
      <span className='text-muted-foreground text-sm'>{props.label}</span>
      <span className='text-sm font-medium break-all'>
        {props.value || '-'}
      </span>
    </div>
  )
}

export function Hermes() {
  const { t } = useTranslation()
  const [tenant, setTenant] = useState<HermesTenant | null>(null)
  const [pairing, setPairing] = useState<HermesPairingSession | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let cancelled = false

    async function load() {
      setLoading(true)
      try {
        const [tenantRes, pairingRes] = await Promise.all([
          getHermesTenantSelf(),
          getLatestHermesPairingSession(),
        ])
        if (!cancelled) {
          setTenant(tenantRes.data?.tenant || null)
          setPairing(pairingRes.data?.session || null)
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    load()
    return () => {
      cancelled = true
    }
  }, [])

  const dashboardUrl = '/api/hermes/tenant/dashboard/'
  const pairingUrl = pairing?.pairing_url

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Hermes Agent')}</SectionPageLayout.Title>
      <SectionPageLayout.Description>
        {t('Open your tenant dashboard and check Feishu pairing status.')}
      </SectionPageLayout.Description>
      <SectionPageLayout.Content>
        <div className='mx-auto grid w-full max-w-6xl gap-4 lg:grid-cols-[minmax(0,1fr)_360px]'>
          <Card>
            <CardHeader>
              <CardTitle className='flex items-center gap-2'>
                <RadioTower className='size-5' />
                {t('Tenant')}
              </CardTitle>
              <CardDescription>
                {t('This NewAPI account maps to one isolated Hermes tenant.')}
              </CardDescription>
              <CardAction>
                {loading ? (
                  <Skeleton className='h-6 w-20' />
                ) : (
                  <StatusBadge value={tenant?.status} />
                )}
              </CardAction>
            </CardHeader>
            <CardContent className='space-y-3'>
              {loading ? (
                <>
                  <Skeleton className='h-10 w-full' />
                  <Skeleton className='h-10 w-full' />
                  <Skeleton className='h-10 w-full' />
                </>
              ) : (
                <>
                  <DetailRow label={t('Tenant ID')} value={tenant?.tenant_id} />
                  <DetailRow
                    label={t('Service Name')}
                    value={tenant?.service_name}
                  />
                  <DetailRow
                    label={t('Public URL')}
                    value={tenant?.public_url}
                  />
                  <DetailRow
                    label={t('Last Health Status')}
                    value={tenant?.last_health_status}
                  />
                </>
              )}
              <div className='flex flex-wrap gap-2 pt-2'>
                <Button asChild>
                  <a href={dashboardUrl} target='_blank' rel='noreferrer'>
                    <ExternalLink className='size-4' />
                    {t('Open Hermes Dashboard')}
                  </a>
                </Button>
                <Button
                  variant='outline'
                  onClick={() => window.location.reload()}
                  disabled={loading}
                >
                  <RefreshCw className='size-4' />
                  {t('Refresh')}
                </Button>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className='flex items-center gap-2'>
                <Link2 className='size-5' />
                {t('Feishu Pairing')}
              </CardTitle>
              <CardDescription>
                {t('Pairing URLs are generated inside the Hermes container.')}
              </CardDescription>
              <CardAction>
                {loading ? (
                  <Skeleton className='h-6 w-20' />
                ) : (
                  <StatusBadge value={pairing?.status} />
                )}
              </CardAction>
            </CardHeader>
            <CardContent className='space-y-3'>
              {loading ? (
                <>
                  <Skeleton className='h-10 w-full' />
                  <Skeleton className='h-24 w-full' />
                </>
              ) : (
                <>
                  <DetailRow label={t('Session ID')} value={pairing?.id} />
                  <DetailRow
                    label={t('Expires At')}
                    value={pairing?.expires_at}
                  />
                  {pairingUrl ? (
                    <Button asChild variant='outline' className='w-full'>
                      <a href={pairingUrl} target='_blank' rel='noreferrer'>
                        <ExternalLink className='size-4' />
                        {t('Open Pairing URL')}
                      </a>
                    </Button>
                  ) : (
                    <div className='text-muted-foreground rounded-lg border border-dashed p-4 text-sm'>
                      {t(
                        'No active pairing URL. Run the controlled delivery pairing step to generate one.'
                      )}
                    </div>
                  )}
                </>
              )}
            </CardContent>
          </Card>
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
