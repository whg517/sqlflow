# SF-FEAT0040 飞书 Webhook 通知通道

> 状态：已通过
> 优先级：P2🟡一般
> 负责人：待分配
> 创建日期：2026-05-28
> 需求编号：recvkUEeWhbYCg

## 需求概述

新增飞书 Webhook 通知通道，支持工单状态变更时发送飞书消息通知。

## 评审结论

- **架构评审（方远）**：✅ 通过（纯后端需求，UI/UX 豁免）

## 技术方案

### 现有功能

| 功能 | 说明 |
|------|------|
| DingTalk Webhook | 已集成，作为参考实现 |
| 通知服务 | `notify_test.go` 测试文件存在 |
| 工单通知触发点 | 已有通知触发机制 |

### 实施方案

#### 架构设计

```
现有通知服务架构：
NotifyService
├── DingTalkProvider  (已有)
├── FeishuProvider    (新增)
└── Provider 接口     (新增抽象)
```

#### 后端实现

| 改动 | 说明 |
|------|------|
| Webhook URL 配置 | 环境变量 `SQLFLOW_FEISHU_WEBHOOK_URL` + 数据库配置 |
| FeishuProvider | 实现 Provider 接口，发送飞书 Interactive Card 消息 |
| 通知事件 | 工单提交、审批通过、审批拒绝 |
| 消息格式 | 飞书 Interactive Card（标题 + 摘要 + 按钮） |
| 错误处理 | HTTP 重试（3 次，指数退避）+ 日志记录 |
| 配置管理 | 支持按事件类型启用/禁用飞书通知 |

#### 飞书消息 Card 示例

```json
{
  "msg_type": "interactive",
  "card": {
    "header": { "title": { "tag": "plain_text", "content": "工单审批通知" } },
    "elements": [
      { "tag": "div", "text": { "tag": "lark_md", "content": "**工单 #123** 已通过审批" } },
      { "tag": "div", "text": { "tag": "lark_md", "content": "提交人：alice\n审批人：bob" } }
    ]
  }
}
```

### 工时估算

| 任务 | 工时 |
|------|------|
| Provider 接口抽象 + FeishuProvider | 1.5h |
| 飞书 Card 消息模板 | 1h |
| 重试机制 + 错误处理 | 0.5h |
| 配置管理 + 集成测试 | 1h |
| **合计** | **4h** |

## 验收标准

1. 环境变量配置 Webhook URL 后，飞书通知正常发送
2. 工单提交、审批通过、审批拒绝三个事件触发飞书通知
3. 飞书 Interactive Card 格式正确，信息完整
4. 发送失败时自动重试（最多 3 次）
5. 不影响现有 DingTalk 通知功能

## Code Review 记录

| 日期 | 审查人 | 结论 | 备注 |
|------|--------|------|------|
| — | — | — | 待开发完成后填写 |

## 变更记录

| 日期 | 变更内容 |
|------|----------|
| 2026-06-13 | 初版创建 |
