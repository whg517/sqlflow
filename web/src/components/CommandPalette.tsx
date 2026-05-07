import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Database,
  FileText,
  ShieldCheck,
  ScrollText,
  Server,
  EyeOff,
  Bot,
} from 'lucide-react'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command'

interface CommandPaletteProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

const pages = [
  { to: '/query', label: '查询', icon: Database, group: '页面跳转' },
  { to: '/tickets', label: '变更工单', icon: FileText, group: '页面跳转' },
  { to: '/permissions', label: '权限管理', icon: ShieldCheck, group: '页面跳转' },
  { to: '/audit', label: '审计日志', icon: ScrollText, group: '页面跳转' },
  { to: '/settings/datasource', label: '数据源管理', icon: Server, group: '设置' },
  { to: '/settings/mask-rules', label: '脱敏规则', icon: EyeOff, group: '设置' },
  { to: '/settings/ai-config', label: 'AI 配置', icon: Bot, group: '设置' },
]

export default function CommandPalette({ open, onOpenChange }: CommandPaletteProps) {
  const navigate = useNavigate()

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        onOpenChange(!open)
      }
    }
    document.addEventListener('keydown', handler)
    return () => document.removeEventListener('keydown', handler)
  }, [open, onOpenChange])

  const runCommand = (command: () => void) => {
    onOpenChange(false)
    command()
  }

  const pageGroup = pages.filter((p) => p.group === '页面跳转')
  const settingsGroup = pages.filter((p) => p.group === '设置')

  return (
    <CommandDialog
      open={open}
      onOpenChange={onOpenChange}
      title="全局搜索"
      description="搜索页面或功能..."
    >
      <CommandInput placeholder="搜索页面或功能..." />
      <CommandList>
        <CommandEmpty>没有找到匹配项</CommandEmpty>
        <CommandGroup heading="页面">
          {pageGroup.map((page) => (
            <CommandItem
              key={page.to}
              onSelect={() => runCommand(() => navigate(page.to))}
            >
              <page.icon size={16} />
              <span>{page.label}</span>
            </CommandItem>
          ))}
        </CommandGroup>
        <CommandSeparator />
        <CommandGroup heading="设置">
          {settingsGroup.map((page) => (
            <CommandItem
              key={page.to}
              onSelect={() => runCommand(() => navigate(page.to))}
            >
              <page.icon size={16} />
              <span>{page.label}</span>
            </CommandItem>
          ))}
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  )
}
