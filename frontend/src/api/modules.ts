import { get } from './request'

export interface ModuleStatus {
  name: string
  version: string
  description: string
  enabled: boolean
  healthy: boolean
  health_error?: string
  multi_instance?: string
  details?: Record<string, unknown>
}

interface ModulesResponse {
  code: number
  data: ModuleStatus[]
}

interface ModuleResponse {
  code: number
  data: ModuleStatus
}

export const listModules = () => get<ModulesResponse>('/modules')

export const getModule = (name: string) => get<ModuleResponse>(`/modules/${name}`)
