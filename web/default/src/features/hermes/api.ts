import { api } from '@/lib/api'
import type {
  HermesPairingSelfData,
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
