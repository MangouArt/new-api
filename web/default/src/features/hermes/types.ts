export type HermesTenantResponse<T> = {
  success: boolean
  message?: string
  data?: T
}

export type HermesTenantSelfData = {
  tenant: HermesTenant | null
  exists: boolean
}

export type HermesPairingSelfData = {
  session: HermesPairingSession | null
  exists: boolean
}

export type HermesTenantAdminListData = {
  page: number
  page_size: number
  total: number
  items: HermesTenantUser[]
}

export type HermesTenantUser = {
  user_id: number
  username: string
  display_name?: string
  email?: string
  role: number
  status: number
  group?: string
  created_at?: number
  tenant: HermesTenant | null
  latest_pairing: HermesPairingSession | null
}

export type HermesTenant = {
  id: number
  user_id: number
  tenant_token_id: number
  tenant_id: string
  service_name: string
  volume_name: string
  status: string
  zeabur_project_id?: string
  zeabur_environment_id?: string
  zeabur_service_id?: string
  zeabur_volume_id?: string
  public_url?: string
  dashboard_url?: string
  last_health_status?: string
  last_health_checked_at?: number
  created_at?: number
  updated_at?: number
}

export type HermesPairingSession = {
  id: number
  tenant_id: number
  user_id: number
  status: string
  pairing_url?: string
  command_exit_code?: number
  command_output_summary?: string
  expires_at?: number
  completed_at?: number
  created_at?: number
  updated_at?: number
}
