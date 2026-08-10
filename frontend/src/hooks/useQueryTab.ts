import { useCallback, useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'

/** Sync Ant Design Tabs with `?tab=` query. Default key omits the param. */
export function useQueryTab(allowed: readonly string[], defaultKey: string) {
  const [searchParams, setSearchParams] = useSearchParams()

  const activeTab = useMemo(() => {
    const tab = searchParams.get('tab')
    return tab && allowed.includes(tab) ? tab : defaultKey
  }, [searchParams, allowed, defaultKey])

  const setActiveTab = useCallback((key: string) => {
    setSearchParams(key === defaultKey ? {} : { tab: key }, { replace: true })
  }, [defaultKey, setSearchParams])

  return [activeTab, setActiveTab] as const
}
