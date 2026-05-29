# SF-ENG0028 MongoDB Aggregation Pipeline 白名单扩展

> 状态：开发中
> 优先级：P3🔵低优
> 负责人：陈岩
> 创建日期：2026-05-27
> 需求编号：recvkMoLiWQ9T3

## 需求概述

扩展 MongoDB aggregation pipeline 支持的 stage 类型，从 9 个扩展到 40+ 个，增加黑名单机制。

## 评审结论

待评审。

## 技术方案

### 架构设计

- 白名单从 9 个扩展到 40+ 个安全 stage
- 新增显式黑名单 `blockedAggStages`（$out/$merge/$currentOp/$listSessions/$changeStream）
- 验证逻辑：先检查黑名单 → 再检查白名单 → 未知 stage 视为 dangerous

### 白名单分类

| 分类 | 新增 Stage |
|------|-----------|
| Grouping | $bucket, $bucketAuto, $densify, $fill |
| Projection | $set, $unset, $unsetField, $setField, $replaceRoot, $replaceWith, $setWindowFields |
| Joining | $lookup, $graphLookup |
| Faceted | $facet |
| Statistical/Geo | $geoNear, $near, $nearSphere, $sample |
| Search | $search, $searchMeta, $vectorSearch |
| Expression | $documents, $sortArray, $reduce, $map, $filter |
| Meta | $collStats, $indexStats, $planCacheStats |

### 黑名单

| Stage | 原因 |
|-------|------|
| $out | 写入 collection |
| $merge | 写入 collection |
| $currentOp | 暴露系统操作 |
| $listLocalSessions | 会话信息泄露 |
| $listSessions | 会话信息泄露 |
| $changeStream | 长连接流，不适合即时查询场景 |

## 验收标准

1. 40+ 个 stage 可正常使用
2. 黑名单 stage 仍被阻止
3. 未知 stage 视为 dangerous
4. 测试覆盖新增 stage + 黑名单验证

## Code Review 记录

| 日期 | 审查人 | 结论 | 备注 |
|------|--------|------|------|
| 2026-05-27 | Marcus | ✅ 通过 | 无阻塞项，NICE: $search/$vectorSearch 需 Atlas Search |

## 变更记录

| 日期 | 变更内容 |
|------|----------|
| 2026-05-27 | 初版创建 |
