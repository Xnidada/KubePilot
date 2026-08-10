import { useEffect, useRef } from 'react'

/** Call `fn` on an interval while `enabled` is true. Always invokes latest `fn`. */
export function useInterval(fn: () => void, ms: number, enabled: boolean) {
  const fnRef = useRef(fn)
  fnRef.current = fn

  useEffect(() => {
    if (!enabled) return
    const id = window.setInterval(() => fnRef.current(), ms)
    return () => window.clearInterval(id)
  }, [ms, enabled])
}
