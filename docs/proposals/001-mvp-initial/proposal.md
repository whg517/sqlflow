# Proposal: SQLFlow MVP 初始开发

> 状态: 已批准
> 创建日期: 2026-04-21
> 更新日期: 2026-05-02
> 关联 Spec: spec/PRD.md, spec/ARCHITECTURE.md, spec/UI-DESIGN.md

## 概述

SQL 审批管理平台 MVP 版本开发，让开发团队大部分低风险查询自助化，高风险操作走审批流程，全程留痕可追溯。将运维从"执行者"转变为"审批者和规则制定者"。

## 背景

公司开发团队频繁找运维执行 SQL，原因：
- 需要运维监管，管控增删改等危险操作
- 避免开发人员查看或导出敏感数据
- 现状为口头面对面沟通，无审批流程和记录

## 范围

- **数据库**：MySQL + MongoDB（仅生产环境）
- **不纳入**：Elasticsearch、INSERT 操作、钉钉 OAuth、refresh token、HTTPS、审计日志防篡改
- **7 个 Phase**，约 10.5 工作日
- 详见 `plan.md`

## Spec 变更

- 首次创建 Spec，无前置变更

## 风险与依赖

- Claude Code 单次任务可能超时（复杂 Task 需拆分）
- pingcap/parser Go 版本兼容性需预先验证
- SSE 流式前后端对接复杂度较高
- Casbin Ent Adapter 成熟度待验证

## 验收标准

1. 全新环境 `docker-compose up -d` 一键启动，无报错
2. 管理员登录 → 注册 MySQL 数据源 → 测试连接返回 200 → 为开发人员分配 `select` 权限
3. 开发人员登录 → 执行 `SELECT` 返回结果表格（≤1000 行）→ 导出 CSV 文件可下载
4. 开发人员提交 DDL 工单 → AI 评审返回风险等级 → DBA 审批通过 → 手动执行成功
5. 查询含敏感字段（手机号/身份证）的结果自动脱敏为 `138****5678` / `310***********1234`
6. 审计日志页可按用户/操作类型/日期筛选，覆盖 SELECT/DDL/DML/EXPORT 四种操作
7. 钉钉 Webhook URL 配置后，工单状态变更时收到通知消息
