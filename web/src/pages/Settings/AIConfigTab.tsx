import { useState, useEffect, useCallback, type FormEvent } from 'react'
import { toast } from 'sonner'
import { Brain, Bell, Loader2, CheckCircle2, XCircle, Send } from 'lucide-react'
import { api } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from '@/components/ui/select'

// --- Types ---

interface AIConfig {
  provider: string
  model: string
  api_key: string
  base_url: string
  timeout: string
  enabled: boolean
}

interface DingTalkConfig {
  webhook_url: string
  secret: string
  enabled: boolean
}

interface SettingsResponse {
  code: number
  data: {
    ai: AIConfig
    dingtalk: DingTalkConfig
  }
}

interface ApiResponse {
  code: number
  message: string
  data: unknown
}

const PROVIDER_OPTIONS = [
  { value: 'openai', label: 'OpenAI' },
  { value: 'zhipu', label: '智谱 GLM' },
  { value: 'azure', label: 'Azure OpenAI' },
  { value: 'custom', label: '自定义 (OpenAI 兼容)' },
]

const MODEL_OPTIONS: Record<string, { value: string; label: string }[]> = {
  openai: [
    { value: 'gpt-4', label: 'GPT-4' },
    { value: 'gpt-4o', label: 'GPT-4o' },
    { value: 'gpt-4o-mini', label: 'GPT-4o Mini' },
    { value: 'gpt-3.5-turbo', label: 'GPT-3.5 Turbo' },
    { value: 'custom', label: '自定义模型' },
  ],
  zhipu: [
    { value: 'glm-4', label: 'GLM-4' },
    { value: 'glm-4-flash', label: 'GLM-4 Flash' },
    { value: 'glm-4-plus', label: 'GLM-4 Plus' },
    { value: 'glm-4-air', label: 'GLM-4 Air' },
    { value: 'glm-4-long', label: 'GLM-4 Long' },
    { value: 'custom', label: '自定义模型' },
  ],
  azure: [
    { value: 'gpt-4', label: 'GPT-4' },
    { value: 'gpt-4o', label: 'GPT-4o' },
    { value: 'gpt-4o-mini', label: 'GPT-4o Mini' },
    { value: 'custom', label: '自定义模型' },
  ],
  custom: [
    { value: 'custom', label: '输入模型名称' },
  ],
}

const DEFAULT_BASE_URLS: Record<string, string> = {
  openai: 'https://api.openai.com/v1',
  zhipu: 'https://open.bigmodel.cn/api/paas/v4',
  azure: '',
  custom: '',
}

const TIMEOUT_OPTIONS = [
  { value: '5s', label: '5 秒' },
  { value: '10s', label: '10 秒（默认）' },
  { value: '15s', label: '15 秒' },
  { value: '20s', label: '20 秒' },
  { value: '30s', label: '30 秒' },
  { value: '60s', label: '60 秒' },
]

// --- AI Config Section ---

function AIConfigSection() {
  const [config, setConfig] = useState<AIConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  const [provider, setProvider] = useState('openai')
  const [model, setModel] = useState('gpt-4')
  const [customModel, setCustomModel] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [baseUrl, setBaseUrl] = useState('https://api.openai.com/v1')
  const [timeout, setTimeout_] = useState('10s')

  // Current provider's model options
  const currentModelOptions = MODEL_OPTIONS[provider] || MODEL_OPTIONS.custom

  const fetchConfig = useCallback(async () => {
    try {
      const res = await api.get<SettingsResponse>('/settings')
      const ai = res.data?.ai
      if (ai) {
        setConfig(ai)
        setProvider(ai.provider || 'openai')
        const opts = MODEL_OPTIONS[ai.provider || 'openai'] || MODEL_OPTIONS.custom
        const isPreset = opts.some((o) => o.value === ai.model)
        if (isPreset) {
          setModel(ai.model)
          setCustomModel('')
        } else {
          setModel('custom')
          setCustomModel(ai.model)
        }
        setApiKey('')
        setBaseUrl(ai.base_url || DEFAULT_BASE_URLS[ai.provider || 'openai'] || 'https://api.openai.com/v1')
        setTimeout_(ai.timeout || '10s')
      }
    } catch {
      toast.error('获取 AI 配置失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchConfig() }, [fetchConfig])

  async function handleSave(e: FormEvent) {
    e.preventDefault()
    setSaving(true)
    try {
      const actualModel = model === 'custom' ? customModel.trim() : model
      if (!actualModel) {
        toast.error('请输入模型名称')
        return
      }
      await api.put<ApiResponse>('/settings/ai', {
        provider,
        model: actualModel,
        api_key: apiKey,
        base_url: baseUrl.trim(),
        timeout,
      })
      toast.success('AI 配置已保存')
      fetchConfig()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="flex h-32 items-center justify-center">
        <Loader2 className="h-5 w-5 animate-spin text-[var(--text-muted)]" />
      </div>
    )
  }

  const resolvedModel = model === 'custom' ? customModel : model

  return (
    <form onSubmit={handleSave} className="space-y-6">
      {/* Status indicator */}
      <div className="flex items-center gap-3 rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] p-4">
        {config?.enabled ? (
          <>
            <CheckCircle2 size={18} className="text-emerald-400" />
            <span className="text-sm text-[var(--text-primary)]">
              AI 评审已启用 — 模型: <span className="font-mono font-medium">{resolvedModel || config?.model}</span>
            </span>
          </>
        ) : (
          <>
            <XCircle size={18} className="text-[var(--text-muted)]" />
            <span className="text-sm text-[var(--text-secondary)]">
              AI 评审未启用 — 请配置 API Key 后启用
            </span>
          </>
        )}
      </div>

      {/* Provider & Model */}
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-1.5">
          <Label className="text-[var(--text-secondary)]">AI 服务商</Label>
          <Select value={provider} onValueChange={setProvider}>
            <SelectTrigger className="border-[var(--border-default)] bg-[var(--bg-elevated)]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {PROVIDER_OPTIONS.map((o) => (
                <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1.5">
          <Label className="text-[var(--text-secondary)]">模型</Label>
          {model !== 'custom' ? (
            <Select value={model} onValueChange={setModel}>
              <SelectTrigger className="border-[var(--border-default)] bg-[var(--bg-elevated)]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {currentModelOptions.map((o) => (
                  <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : (
            <div className="flex gap-2">
              <Input
                value={customModel}
                onChange={(e) => setCustomModel(e.target.value)}
                placeholder="输入模型名称"
                className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
              />
              <Button
                type="button" variant="outline" size="sm"
                className="shrink-0 border-[var(--border-default)] text-xs"
                onClick={() => { setModel('gpt-4'); setCustomModel('') }}
              >
                预设
              </Button>
            </div>
          )}
          {model === 'custom' && !customModel && (
            <p className="text-xs text-[var(--text-muted)]">输入模型后请选择"预设"切换回下拉列表</p>
          )}
        </div>
      </div>

      {/* Base URL */}
      <div className="space-y-1.5">
        <Label className="text-[var(--text-secondary)]">API 地址</Label>
        <Input
          value={baseUrl}
          onChange={(e) => setBaseUrl(e.target.value)}
          placeholder="https://api.openai.com/v1"
          className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
        />
        <p className="text-xs text-[var(--text-muted)]">
          OpenAI 兼容的 Chat Completions API 地址
        </p>
      </div>

      {/* API Key */}
      <div className="space-y-1.5">
        <Label className="text-[var(--text-secondary)]">
          API Key{' '}
          {config?.api_key && (
            <span className="font-normal text-[var(--text-muted)]">
              (当前: <span className="font-mono">{config.api_key}</span>)
            </span>
          )}
        </Label>
        <Input
          type="password"
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
          placeholder={config?.api_key ? '留空保持当前 Key 不变' : '输入 API Key 以启用 AI 评审'}
          className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
        />
        {!config?.enabled && !apiKey && (
          <p className="text-xs text-yellow-400">未配置 API Key，AI 评审将使用静态规则</p>
        )}
      </div>

      {/* Timeout */}
      <div className="space-y-1.5">
        <Label className="text-[var(--text-secondary)]">评审超时时间</Label>
        <Select value={timeout} onValueChange={setTimeout_}>
          <SelectTrigger className="w-48 border-[var(--border-default)] bg-[var(--bg-elevated)]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {TIMEOUT_OPTIONS.map((o) => (
              <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        <p className="text-xs text-[var(--text-muted)]">超时后将自动降级为静态规则评审</p>
      </div>

      {/* Save */}
      <div className="flex items-center gap-3 pt-2">
        <Button
          type="submit" disabled={saving}
          className="bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
        >
          {saving ? '保存中...' : '保存 AI 配置'}
        </Button>
      </div>
    </form>
  )
}

// --- DingTalk Config Section ---

function DingTalkSection() {
  const [config, setConfig] = useState<DingTalkConfig | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)

  const [webhookUrl, setWebhookUrl] = useState('')
  const [secret, setSecret] = useState('')

  const fetchConfig = useCallback(async () => {
    try {
      const res = await api.get<SettingsResponse>('/settings')
      const dt = res.data?.dingtalk
      if (dt) {
        setConfig(dt)
        setWebhookUrl(dt.webhook_url || '')
        setSecret('')
      }
    } catch {
      toast.error('获取通知配置失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchConfig() }, [fetchConfig])

  async function handleSave(e: FormEvent) {
    e.preventDefault()
    setSaving(true)
    try {
      await api.put<ApiResponse>('/settings/dingtalk', {
        webhook_url: webhookUrl.trim(),
        secret: secret.trim(),
      })
      toast.success('钉钉通知配置已保存')
      fetchConfig()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  async function handleTest() {
    setTesting(true)
    try {
      await api.post<ApiResponse>('/settings/dingtalk/test', {})
      toast.success('测试消息已发送，请检查钉钉群')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '测试发送失败')
    } finally {
      setTesting(false)
    }
  }

  if (loading) {
    return (
      <div className="flex h-32 items-center justify-center">
        <Loader2 className="h-5 w-5 animate-spin text-[var(--text-muted)]" />
      </div>
    )
  }

  return (
    <form onSubmit={handleSave} className="space-y-6">
      {/* Status indicator */}
      <div className="flex items-center gap-3 rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] p-4">
        {config?.enabled ? (
          <>
            <CheckCircle2 size={18} className="text-emerald-400" />
            <span className="text-sm text-[var(--text-primary)]">钉钉通知已启用</span>
          </>
        ) : (
          <>
            <XCircle size={18} className="text-[var(--text-muted)]" />
            <span className="text-sm text-[var(--text-secondary)]">钉钉通知未启用 — 请配置 Webhook URL</span>
          </>
        )}
      </div>

      {/* Webhook URL */}
      <div className="space-y-1.5">
        <Label className="text-[var(--text-secondary)]">Webhook URL</Label>
        <Input
          value={webhookUrl}
          onChange={(e) => setWebhookUrl(e.target.value)}
          placeholder="https://oapi.dingtalk.com/robot/send?access_token=..."
          className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
        />
        <p className="text-xs text-[var(--text-muted)]">
          在钉钉群 → 设置 → 智能群助手 → 添加机器人 → 自定义 获取
        </p>
      </div>

      {/* Secret */}
      <div className="space-y-1.5">
        <Label className="text-[var(--text-secondary)]">
          签名密钥（加签）{' '}
          {config?.secret && (
            <span className="font-normal text-[var(--text-muted)]">
              (当前: <span className="font-mono">{config.secret}</span>)
            </span>
          )}
        </Label>
        <Input
          type="password"
          value={secret}
          onChange={(e) => setSecret(e.target.value)}
          placeholder={config?.secret ? '留空保持当前密钥不变' : '可选，机器人加签密钥'}
          className="border-[var(--border-default)] bg-[var(--bg-elevated)]"
        />
      </div>

      {/* Actions */}
      <div className="flex items-center gap-3 pt-2">
        <Button
          type="submit" disabled={saving}
          className="bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-hover)]"
        >
          {saving ? '保存中...' : '保存通知配置'}
        </Button>
        <Button
          type="button" variant="outline"
          className="gap-1.5 border-[var(--border-default)]"
          disabled={testing || !config?.enabled}
          onClick={handleTest}
        >
          {testing ? <Loader2 size={14} className="animate-spin" /> : <Send size={14} />}
          {testing ? '发送中...' : '发送测试消息'}
        </Button>
      </div>
    </form>
  )
}

// --- Main AIConfigTab ---

export default function AIConfigTab() {
  return (
    <div className="space-y-8">
      {/* AI Configuration */}
      <div className="space-y-4">
        <div className="flex items-center gap-2">
          <Brain size={18} className="text-[var(--accent-primary)]" />
          <h2 className="text-lg font-semibold text-[var(--text-primary)]">AI 评审配置</h2>
        </div>
        <p className="text-sm text-[var(--text-secondary)]">
          配置外部 LLM API，用于 SQL 评审、风险分级和优化建议。
          未配置时系统将使用静态规则引擎。
        </p>
        <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] p-6">
          <AIConfigSection />
        </div>
      </div>

      {/* Divider */}
      <div className="border-t border-[var(--border-subtle)]" />

      {/* DingTalk Notification */}
      <div className="space-y-4">
        <div className="flex items-center gap-2">
          <Bell size={18} className="text-[var(--accent-primary)]" />
          <h2 className="text-lg font-semibold text-[var(--text-primary)]">钉钉通知配置</h2>
          <Badge className="bg-blue-500/20 text-blue-400 text-xs">通知</Badge>
        </div>
        <p className="text-sm text-[var(--text-secondary)]">
          配置钉钉机器人 Webhook，用于工单提交通知、审批结果推送和风险告警。
        </p>
        <div className="rounded-lg border border-[var(--border-default)] bg-[var(--bg-surface)] p-6">
          <DingTalkSection />
        </div>
      </div>
    </div>
  )
}
