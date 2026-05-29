# SF-ENG0033 前端 Core Web Vitals 性能监控

> 状态：已通过
> 优先级：P2🟡一般
> 负责人：待分配
> 创建日期：2026-05-28
> 需求编号：recvkUEjvW4uCw

## 需求概述

集成前端 Core Web Vitals 性能监控，采集 LCP/INP/CLS 指标，建立性能基线。

## 评审结论

- **架构评审（方远）**：✅ 通过（ENG 需求，无需 UI/UX 评审）

## 技术方案

### Core Web Vitals 指标

| 指标 | 全称 | 目标值 | 说明 |
|------|------|--------|------|
| LCP | Largest Contentful Paint | ≤ 2.5s | 最大内容绘制时间 |
| INP | Interaction to Next Paint | ≤ 200ms | 交互到下次绘制时间 |
| CLS | Cumulative Layout Shift | ≤ 0.1 | 累积布局偏移 |

### 实施方案

#### 前端

| 改动 | 说明 |
|------|------|
| `web-vitals` 库 | 安装 `web-vitals` npm 包 |
| 指标采集 | 在 `main.tsx` 中注册 onLCP/onINP/onCLS 回调 |
| 上报逻辑 | 采集后 POST 到 `/api/metrics/web-vitals` |
| 环境判断 | `import.meta.env.PROD` 为 true 时才上报 |
| route 信息 | 附加 `path` 和 `navigation_type` 上下文 |

#### 后端

| 改动 | 说明 |
|------|------|
| API 端点 | `POST /api/metrics/web-vitals`（无需认证，rate limit 保护） |
| 数据表 | `web_vitals`（id, metric_name, value, path, user_agent, created_at） |
| 数据保留 | 30 天自动清理 |

#### 上报数据格式

```typescript
interface WebVitalPayload {
  name: 'LCP' | 'INP' | 'CLS'
  value: number
  rating: 'good' | 'needs-improvement' | 'poor'
  path: string
  navigationType: string
}
```

### 工时估算

| 任务 | 工时 |
|------|------|
| 前端集成 web-vitals + 上报 | 1.5h |
| 后端 API + 数据表 | 1.5h |
| 测试验证 | 1h |
| **合计** | **4h** |

## 验收标准

1. 生产环境页面加载时采集 LCP/INP/CLS 指标
2. 开发环境不上报（`import.meta.env.PROD` 判断）
3. 后端 `/api/metrics/web-vitals` 正确存储指标
4. Rate limit 保护（防滥用）
5. 数据 30 天自动清理
6. 不影响页面加载性能（采集逻辑异步执行）

## Code Review 记录

| 日期 | 审查人 | 结论 | 备注 |
|------|--------|------|------|
| — | — | — | 待开发完成后填写 |

## 变更记录

| 日期 | 变更内容 |
|------|----------|
| 2026-06-13 | 初版创建 |
