import { create } from 'zustand'
import { persist } from 'zustand/middleware'

export interface Permission {
  resource: string
  actions: string[]
}

interface UserInfo {
  id: number
  username: string
  email: string
  real_name: string
  role_id: number
  role_name: string
  is_system?: boolean
  permissions?: Permission[]
}

interface AuthState {
  token: string | null
  user: UserInfo | null
  isAuthenticated: boolean
  setToken: (token: string) => void
  setUser: (user: UserInfo) => void
  logout: () => void
  hasPermission: (resource: string, action?: string) => boolean
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
      token: null,
      user: null,
      isAuthenticated: false,
      setToken: (token: string) => set({ token, isAuthenticated: true }),
      setUser: (user: UserInfo) => set({ user }),
      logout: () => {
        set({ token: null, user: null, isAuthenticated: false })
        localStorage.removeItem('auth-storage')
      },
      hasPermission: (resource: string, action = 'view') => {
        const user = get().user
        if (!user) return false
        if (user.is_system || user.role_name === 'admin') return true
        const permissions = user.permissions || []
        return permissions.some(permission => {
          const resourceMatch = permission.resource === '*' || permission.resource === resource
          if (!resourceMatch) return false
          return permission.actions.includes('*') || permission.actions.includes(action)
        })
      },
    }),
    {
      name: 'auth-storage',
    }
  )
)
