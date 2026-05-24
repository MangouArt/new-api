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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { SectionPageLayout } from '@/components/layout'
import {
  deployHermesTenantForUser,
  getHermesTenantAdminList,
  getHermesTenantSelf,
  getLatestHermesPairingSession,
} from './api'
import type {
  HermesPairingSession,
  HermesTenant,
  HermesTenantUser,
} from './types'

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
  const [tenantUsers, setTenantUsers] = useState<HermesTenantUser[] | null>(
    null
  )
  const [deployingUserID, setDeployingUserID] = useState<number | null>(null)
  const [loading, setLoading] = useState(true)

  async function reload() {
    setLoading(true)
    try {
      const [tenantRes, pairingRes, adminListRes] = await Promise.all([
        getHermesTenantSelf(),
        getLatestHermesPairingSession(),
        getHermesTenantAdminList().catch(() => null),
      ])
      setTenant(tenantRes.data?.tenant || null)
      setPairing(pairingRes.data?.session || null)
      setTenantUsers(adminListRes?.data?.items || null)
    } finally {
      setLoading(false)
    }
  }

  async function deployForUser(userID: number) {
    setDeployingUserID(userID)
    try {
      await deployHermesTenantForUser(userID)
      await reload()
    } finally {
      setDeployingUserID(null)
    }
  }

  useEffect(() => {
    let cancelled = false

    async function load() {
      setLoading(true)
      try {
        const [tenantRes, pairingRes, adminListRes] = await Promise.all([
          getHermesTenantSelf(),
          getLatestHermesPairingSession(),
          getHermesTenantAdminList().catch(() => null),
        ])
        if (!cancelled) {
          setTenant(tenantRes.data?.tenant || null)
          setPairing(pairingRes.data?.session || null)
          setTenantUsers(adminListRes?.data?.items || null)
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
        <div className='mx-auto flex w-full max-w-6xl flex-col gap-4'>
          {tenantUsers ? (
            <Card>
              <CardHeader>
                <CardTitle>{t('Hermes Tenant Control Plane')}</CardTitle>
                <CardDescription>
                  {t(
                    'Super administrators can inspect every NewAPI user and the linked Hermes tenant status.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className='overflow-x-auto rounded-lg border'>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>{t('User')}</TableHead>
                        <TableHead>{t('Tenant')}</TableHead>
                        <TableHead>{t('Runtime')}</TableHead>
                        <TableHead>{t('Pairing')}</TableHead>
                        <TableHead>{t('Dashboard')}</TableHead>
                        <TableHead>{t('Actions')}</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {tenantUsers.length === 0 ? (
                        <TableRow>
                          <TableCell
                            colSpan={6}
                            className='text-muted-foreground h-24 text-center'
                          >
                            {t('No users found.')}
                          </TableCell>
                        </TableRow>
                      ) : (
                        tenantUsers.map((item) => (
                          <TableRow key={item.user_id}>
                            <TableCell>
                              <div className='font-medium'>
                                {item.display_name || item.username}
                              </div>
                              <div className='text-muted-foreground text-xs'>
                                #{item.user_id} {item.email || item.username}
                              </div>
                            </TableCell>
                            <TableCell>
                              <div className='flex flex-col gap-1'>
                                <StatusBadge value={item.tenant?.status} />
                                <span className='text-muted-foreground text-xs'>
                                  {item.tenant?.tenant_id || '-'}
                                </span>
                              </div>
                            </TableCell>
                            <TableCell className='min-w-44'>
                              <div className='text-sm break-all'>
                                {item.tenant?.service_name || '-'}
                              </div>
                              <div className='text-muted-foreground text-xs break-all'>
                                {item.tenant?.zeabur_service_id || ''}
                              </div>
                            </TableCell>
                            <TableCell>
                              <div className='flex flex-col gap-1'>
                                <StatusBadge
                                  value={item.latest_pairing?.status}
                                />
                                <span className='text-muted-foreground text-xs'>
                                  {item.latest_pairing?.id
                                    ? `#${item.latest_pairing.id}`
                                    : '-'}
                                </span>
                              </div>
                            </TableCell>
                            <TableCell>
                              {item.tenant?.public_url ? (
                                <Button asChild variant='outline' size='sm'>
                                  <a
                                    href={item.tenant.public_url}
                                    target='_blank'
                                    rel='noreferrer'
                                  >
                                    <ExternalLink className='size-4' />
                                    {t('Open')}
                                  </a>
                                </Button>
                              ) : (
                                <span className='text-muted-foreground text-sm'>
                                  -
                                </span>
                              )}
                            </TableCell>
                            <TableCell>
                              <Button
                                variant='outline'
                                size='sm'
                                disabled={deployingUserID === item.user_id}
                                onClick={() => deployForUser(item.user_id)}
                              >
                                {deployingUserID === item.user_id
                                  ? t('Deploying')
                                  : item.tenant
                                    ? t('Repair Deploy')
                                    : t('Deploy')}
                              </Button>
                            </TableCell>
                          </TableRow>
                        ))
                      )}
                    </TableBody>
                  </Table>
                </div>
              </CardContent>
            </Card>
          ) : null}

          <div className='grid gap-4 lg:grid-cols-[minmax(0,1fr)_360px]'>
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
                    <DetailRow
                      label={t('Tenant ID')}
                      value={tenant?.tenant_id}
                    />
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
        </div>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
