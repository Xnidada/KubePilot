import { post, get } from './request'

interface LoginRequest {
  username: string
  password: string
}

interface LoginResponse {
  code: number
  message: string
  data: {
    token: string
    expires_at: string
    user: {
      id: number
      username: string
      email: string
      real_name: string
      role_id: number
      role_name: string
    }
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

export const login = (data: LoginRequest) => {
  return post<LoginResponse>('/auth/login', data)
}

export const getProfile = () => {
  return get<ProfileResponse>('/profile')
}

export const changePassword = (data: { old_password: string; new_password: string }) => {
  return post('/profile/password', data)
}
