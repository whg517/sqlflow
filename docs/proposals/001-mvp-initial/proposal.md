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

1. `docker-compose up -d` 一键启动
2. 管理员登录 → 注册数据源 → 配置权限
3. 开发人员登录 → 执行 SELECT 查询 → 查看结果 → 导出 CSV
4. 开发人员提交 DDL 工单 → AI 评审 → DBA 审批 → 手动执行
5. 敏感数据自动脱敏
6. 审计日志完整记录所有操作
7. 钉钉通知在关键节点触发
