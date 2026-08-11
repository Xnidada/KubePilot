import { post, get } from './request'

interface LoginRequest {
  username: string
  password: string
}

export interface AuthUser {
  id: number
  username: string
  email: string
  real_name: string
  role_id: number
  role_name: string
  is_system?: boolean
  permissions?: Array<{ resource: string; actions: string[] }>
}

interface LoginResponse {
  code: number
  message: string
  data: {
    require_2fa?: boolean
    pending_token?: string
    message?: string
    token?: string
    expires_at?: string
    user?: AuthUser
  }
}

interface ProfileResponse {
  code: number
  data: {
    id: number
    username: string
    email: string
    real_name: string
    role_id: number
    role_name: string
  }
}

interface Verify2FAResponse {
  code: number
  message: string
  data: {
    verified: boolean
    backup_code_used?: boolean
    token: string
    expires_at: string
    user: AuthUser
  }
}

export const login = (data: LoginRequest) => {
  return post<LoginResponse>('/auth/login', data)
}

export const verify2FALogin = (pendingToken: string, code: string) => {
  return post<Verify2FAResponse>('/auth/2fa/verify', { pending_token: pendingToken, code })
}

export const getProfile = () => {
  return get<ProfileResponse>('/profile')
}

export const changePassword = (data: { old_password: string; new_password: string }) => {
  return post('/profile/password', data)
}

export const listPublicOAuthProviders = () => {
  return get<{ code: number; data: Array<{ id: number; provider: string; name: string; enabled: boolean }> }>(
    '/oauth/providers'
  )
}

export const initiateOAuthLogin = (provider: string) => {
  return get<{ code: number; data: { auth_url: string; state: string } }>(`/oauth/${provider}/login`)
}
