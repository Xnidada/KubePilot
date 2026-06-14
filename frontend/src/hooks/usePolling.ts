import { useEffect, useRef, useCallback } from 'react'

interface UsePollingOptions {
  enabled?: boolean
  interval?: number // 毫秒
  onError?: (error: Error) => void
}

/**
 * 自动轮询 Hook
 * 当有 Terminating 状态的资源时自动轮询刷新
 */
export function usePolling(
  fetchFn: () => Promise<void>,
  hasTerminating: boolean,
  options: UsePollingOptions = {}
) {
  const { enabled = true, interval = 3000, onError } = options
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const fetchFnRef = useRef(fetchFn)

  // 更新 fetchFn 引用
  useEffect(() => {
    fetchFnRef.current = fetchFn
  }, [fetchFn])

  const startPolling = useCallback(() => {
    if (timerRef.current) {
      clearInterval(timerRef.current)
    }
    timerRef.current = setInterval(() => {
      fetchFnRef.current().catch((err) => {
        onError?.(err)
      })
    }, interval)
  }, [interval, onError])

  const stopPolling = useCallback(() => {
    if (timerRef.current) {
      clearInterval(timerRef.current)
      timerRef.current = null
    }
  }, [])

  useEffect(() => {
    if (enabled && hasTerminating) {
      startPolling()
    } else {
      stopPolling()
    }

    return () => {
      stopPolling()
    }
  }, [enabled, hasTerminating, startPolling, stopPolling])

  return { startPolling, stopPolling }
}

/**
 * 检查列表中是否有 Terminating 状态的资源
 */
export function hasTerminatingResource(items: any[]): boolean {
  return items.some((item) => item.status === 'Terminating')
}
