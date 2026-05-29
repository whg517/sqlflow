# SF-FEAT0030 工单 SLA 告警（审批超时自动提醒/升级）

> 状态：需修改 → 修改中
> 优先级：P2🟡一般
> 负责人：待分配
> 创建日期：2026-05-27
> 需求编号：recvkMokrui2Oh

## 需求概述

工单审批超时自动告警，防止审批积压。支持 SLA 规则配置、超时提醒、超时升级。

## 评审结论

### 🏗️ 架构评审（方远）：⚠️ 需修改
需补充 ADR（SLA 调度选型、配置存储）+ 升级流程状态机 + 集成方案。

### 🛡️ 安全评审（韩锐）：⚠️ 需修改
- 🔴 P0×2：升级路径 RBAC 校验、SLA 配置独立权限
- 🟠 P1×3：通知防滥用、告警脱敏、调度器健康检查
- 🟡 P2×2：审计日志独立存储、时区 UTC 统一

### 🧪 测试策略评审（叶青）：⚠️ 需修改
- 需确认：节假日 SLA 计算、驳回重提 SLA 重置、多级升级链路、代理机制、批量频率控制
- 性能测试：调度器高并发、通知吞吐量、DB 查询压力
- 边界：超时前一秒审批取消提醒、极值 SLA（365天/1分钟）

### 🎨 UI/UX 评审（苏晴）：⚠️ 需修改
- 需补充：SLA 倒计时组件、工单列表超时状态增强、通知消息模板、SLA 配置页面线框图、边界场景处理

### 修改响应

以下根据四方评审意见补充完整技术方案。

## 技术方案

### ADR-002：SLA 调度选型

**决策：复用现有 Scheduler（time.Timer 轮询），不引入新调度框架**

| 方案 | 优点 | 缺点 | 结论 |
|------|------|------|------|
| 复用现有 Scheduler | 零新增依赖，架构一致 | 精度依赖轮询间隔 | ✅ 采纳 |
| go-co-op/gocron | cron 表达式灵活 | 新增依赖，与现有架构不一致 | ❌ |
| 独立 goroutine + ticker | 简单 | 需要单独管理生命周期 | ❌ |

**调度逻辑**：
- 每 **10 分钟** 检查一次 `PENDING_APPROVAL` 工单
- SLA 阈值：80% 时限 → 提醒，100% 时限 → 升级
- 同一工单同一阶段不重复通知（检查 sla_notifications 表）

### ADR-003：SLA 配置存储

**决策：SQLite 表存储（非配置文件）**

| 方案 | 优点 | 缺点 | 结论 |
|------|------|------|------|
| SQLite 表 | 运行时可修改，API 管理方便 | - | ✅ 采纳 |
| config.yaml | 简单 | 修改需重启，不支持多规则 | ❌ |

### 数据库变更

**sla_config 表**：

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | 主键 |
| priority | TEXT NOT NULL | 工单优先级（p0/p1/p2/p3） |
| timeout_minutes | INTEGER NOT NULL | 审批时限（分钟） |
| reminder_percent | INTEGER DEFAULT 80 | 提醒阈值（百分比） |
| escalate_to_role | TEXT | 升级通知角色（admin/dba） |
| escalate_to_user | TEXT | 升级通知指定用户（open_id，优先级高于 role） |
| enabled | BOOLEAN DEFAULT true | 是否启用 |
| created_at | DATETIME | 创建时间 |
| updated_at | DATETIME | 更新时间 |

**sla_notifications 表**（审计日志）：

| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | 主键 |
| ticket_id | INTEGER NOT NULL | 关联工单 |
| notification_type | TEXT NOT NULL | reminder/escalate |
| stage | TEXT NOT NULL | PENDING_APPROVAL |
| notified_user | TEXT NOT NULL | 被通知人 user_id |
| notified_at | DATETIME | 通知时间（UTC） |
| sla_config_id | INTEGER | 使用的 SLA 规则 |

**tickets 表扩展**：

```sql
ALTER TABLE tickets ADD COLUMN sla_deadline DATETIME;       -- SLA 截止时间（UTC）
ALTER TABLE tickets ADD COLUMN sla_status TEXT DEFAULT 'normal'; -- normal/warning/breached
```

### 升级流程状态机

```
工单提交 → PENDING_APPROVAL
  │
  ├─ 80% 时限 → sla_status = warning → 钉钉提醒审批人
  │                                  → 记录 sla_notifications
  │
  ├─ 100% 时限 → sla_status = breached → 钉钉升级通知（escalate_to 指定人）
  │                                     → 记录 sla_notifications
  │
  ├─ 审批人处理 → sla_status = normal → 清除 deadline
  │
  └─ 被驳回 → 驳回重提时 SLA 重置（重新计算 deadline）
```

### 安全设计

| 需求 | 实现 |
|------|------|
| 🔴 升级路径 RBAC | 仅有 admin 角色可配置 SLA 规则、查看 sla_notifications |
| 🔴 SLA 配置独立权限 | 新增 Casbin policy：`admin, sla_config, manage, allow` |
| 🟠 通知防滥用 | 同一工单同一阶段（reminder/escalate）各限 3 次/天 |
| 🟠 告警脱敏 | 钉钉通知不含 SQL 内容、数据库凭据，仅含工单编号+标题+提交人 |
| 🟠 调度器健康检查 | Scheduler 记录最后执行时间，超过 30 分钟未执行 → 写入系统日志 WARNING |
| 🟡 审计日志 | SLA 通知记录入 sla_notifications 表（独立于 audit_logs，避免审计日志膨胀） |
| 🟡 时区 UTC | 所有 sla_deadline / notified_at 使用 UTC 存储，前端展示转换用户时区 |

### 接口设计

| 接口 | 方法 | 说明 | 权限 |
|------|------|------|------|
| /api/settings/sla | GET | 获取 SLA 规则列表 | admin |
| /api/settings/sla | POST | 创建 SLA 规则 | admin |
| /api/settings/sla/:id | PUT | 更新 SLA 规则 | admin |
| /api/settings/sla/:id | DELETE | 删除 SLA 规则 | admin |
| /api/tickets/sla-status | GET | 批量获取工单 SLA 状态 | 所有角色 |
| /api/sla-notifications | GET | SLA 通知记录（分页） | admin |

### 钉钉通知消息模板

**提醒通知（80% 时限）**：
```
⏰ [SQLFlow] 工单审批提醒
工单：#{ticket_id} {ticket_title}
提交人：{submitter}
已等待：{elapsed_hours}h / 时限：{sla_hours}h
请及时处理 👉 {ticket_link}
```

**升级通知（100% 时限）**：
```
🚨 [SQLFlow] 工单审批超时升级
工单：#{ticket_id} {ticket_title}
提交人：{submitter}
超时时限：{sla_hours}h
审批人：{approver}（已提醒未处理）
请立即处理 👉 {ticket_link}
```

### 边界场景处理

| 场景 | 处理 |
|------|------|
| 超时前一秒审批 | 调度器检查时如果工单已不在 PENDING_APPROVAL 状态，跳过通知 |
| 极值 SLA（365天/1分钟） | SLA 时限范围限制：1分钟 ~ 720小时（30天），前端输入校验 |
| 节假日/非工作时间 | V1 不区分工作日/节假日，SLA 按自然时间计算 |
| 驳回重提 SLA 重置 | 工单重新进入 PENDING_APPROVAL 时，sla_deadline 重新计算 |
| 代理审批 | 审批人请假时，通过 escalate_to_role/user 配置升级路径 |
| 批量频率控制 | 同一工单 reminder 限 3 次/天，escalate 限 1 次/天 |
| 调度器重启 | 重启后首次执行检查所有 PENDING_APPROVAL 工单（幂等，检查 sla_notifications 防重） |

### 前端设计

- **SLA 倒计时组件**：工单列表/详情显示 `剩余 2h 15m`，超时后显示 `已超时 30m`（红色）
- **SLA 状态颜色**：🟢 normal / 🟡 warning / 🔴 breached
- **Settings → SLA 配置页**：表格展示规则 + 新增/编辑弹窗
- **Dashboard**：新增「超时工单」统计卡片

### 性能考虑

- SLA 检查查询：`SELECT * FROM tickets WHERE status = 'PENDING_APPROVAL' AND sla_deadline IS NOT NULL`
- 走 `status` 索引，数据量可控（PENDING_APPROVAL 工单通常 < 100）
- 通知发送异步化，不阻塞调度循环

## 验收标准

1. SLA 规则可配置（按优先级设置时限，admin 权限）
2. 80% 时限自动钉钉提醒审批人
3. 100% 时限自动钉钉升级通知
4. 同一阶段不重复通知（防滥用）
5. 工单列表显示 SLA 状态（normal/warning/breached）
6. Dashboard 超时统计
7. 驳回重提 SLA 重置
8. 通知消息不含敏感信息（脱敏）
9. 时区 UTC 存储，前端正确展示
10. 调度器健康检查日志

## 变更记录

| 日期 | 变更内容 |
|------|----------|
| 2026-05-27 | 初版创建 |
| 2026-05-27 | 根据四方评审补充：ADR（调度选型/配置存储）、升级状态机、安全设计（RBAC/防滥用/脱敏）、边界场景、通知模板、性能考虑、前端设计 |
