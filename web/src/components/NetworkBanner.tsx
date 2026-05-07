import { useEffect, useState } from 'react'
import { WifiOff } from 'lucide-react'

export default function NetworkBanner() {
  const [offline, setOffline] = useState(!navigator.onLine)

  useEffect(() => {
    const goOffline = () => setOffline(true)
    const goOnline = () => setOffline(false)
    window.addEventListener('offline', goOffline)
    window.addEventListener('online', goOnline)
    return () => {
      window.removeEventListener('offline', goOffline)
      window.removeEventListener('online', goOnline)
    }
  }, [])

  if (!offline) return null

  return (
    <div className="fixed inset-x-0 top-0 z-[60] flex items-center justify-center gap-2 bg-[var(--risk-high)] px-4 py-2 text-sm font-medium text-white">
      <WifiOff size={16} />
      <span>网络连接已断开，部分功能不可用</span>
    </div>
  )
}
