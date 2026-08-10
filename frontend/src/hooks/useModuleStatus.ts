import { useCallback, useEffect, useState } from 'react'
import { getModule, ModuleStatus } from '../api/modules'

/** Poll a single module status from GET /modules/:name. */
export function useModuleStatus(name: string, refreshMs = 15000) {
  const [status, setStatus] = useState<ModuleStatus | null>(null)

  const refresh = useCallback(async () => {
    try {
      const res = await getModule(name)
      setStatus(res.data || null)
    } catch {
      // ignore — page can still work without banner
    }
  }, [name])

  useEffect(() => {
    refresh()
    if (refreshMs <= 0) return
    const id = window.setInterval(refresh, refreshMs)
    return () => window.clearInterval(id)
  }, [refresh, refreshMs])

  return status
}
