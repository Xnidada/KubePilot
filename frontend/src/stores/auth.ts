import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface UserInfo {
  id: number
  username: string
  email: string
  real_name: string
  role_id: number
  role_name: string
}

interface AuthState {
  token: string | null
  user: UserInfo | null
  isAuthenticated: boolean
  setToken: (token: string) => void
  setUser: (user: UserInfo) => void
  logout: () => void
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      isAuthenticated: false,
      setToken: (token: string) => set({ token, isAuthenticated: true }),
      setUser: (user: UserInfo) => set({ user }),
      logout: () => {
        set({ token: null, user: null, isAuthenticated: false })
        localStorage.removeItem('auth-storage')
      },
    }),
    {
      name: 'auth-storage',
    }
  )
)
