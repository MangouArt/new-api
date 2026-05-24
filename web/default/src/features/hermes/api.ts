import { api } from '@/lib/api'
import type {
  HermesPairingSelfData,
  HermesTenantAdminListData,
  HermesTenantSelfData,
  HermesTenantResponse,
} from './types'

export async function getHermesTenantSelf(): Promise<
  HermesTenantResponse<HermesTenantSelfData>
> {
  const res = await api.get('/api/hermes/tenant/self')
  return res.data
}

export async function getLatestHermesPairingSession(): Promise<
  HermesTenantResponse<HermesPairingSelfData>
> {
  const res = await api.get('/api/hermes/tenant/pairing/latest')
  return res.data
}

export async function getHermesTenantAdminList(): Promise<
  HermesTenantResponse<HermesTenantAdminListData>
> {
  const res = await api.get('/api/hermes/tenants', {
    params: { p: 1, page_size: 100 },
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

export async function deployHermesTenantForUser(
  userId: number
): Promise<HermesTenantResponse<{ tenant: unknown; zeabur: unknown }>> {
  const res = await api.post(`/api/hermes/tenants/user/${userId}/deploy`)
  return res.data
}
