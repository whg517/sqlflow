# Changelog

All notable changes to SQLFlow will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2026-05-23

### Added

**核心功能**
- 数据源管理：支持 MySQL、MongoDB，连接池管理，连接测试
- 在线 SQL 查询：CodeMirror SQL 编辑器，查询历史记录
- 数据导出：查询结果导出为 CSV
- AI 前置评审：提交工单前自动 SQL 安全审查
- 数据脱敏：自定义脱敏规则，查询结果自动脱敏（手机号、身份证、邮箱等）
- 变更工单：SQL 变更审批流程，支持待审批/已批准/已拒绝/已执行/已撤销状态
- 用户管理：用户 CRUD，角色分配（admin / dba / developer）
- RBAC 权限控制：基于 Casbin，按数据源 + 表 + 操作维度精细控制
- 操作审计：全量操作日志记录
- 钉钉通知：工单状态变更消息推送
- Dashboard：数据概览

**前端**
- React 19 + TypeScript 5 技术栈
- Tailwind CSS 4 + Radix UI 组件库
- 登录/注册、查询页面、工单管理、审计日志、系统设置
- 响应式布局

**后端**
- Go 1.25 + Echo v4 框架
- SQLite 数据库（modernc.org/sqlite，纯 Go 实现）
- JWT 认证 + bcrypt 密码哈希
- AES-GCM 数据源密码加密
- Casbin RBAC 权限引擎

**基础设施**
- Dockerfile 多阶段构建
- docker-compose.yaml 容器编排
- 配置文件模板（config.yaml.example）
- 环境变量模板（.env.example）

### Security
- JWT secret 默认值从硬编码 "change-me" 改为自动生成随机密钥，生产环境强制配置
- CORS 配置支持通过环境变量限制允许的源
- Dockerfile 从 golang:1.24 升级到 golang:1.25 匹配 go.mod
- 移除 docker-compose 中的硬编码默认密码

[1.0.0]: https://github.com/whg517/sqlflow/releases/tag/v1.0.0
