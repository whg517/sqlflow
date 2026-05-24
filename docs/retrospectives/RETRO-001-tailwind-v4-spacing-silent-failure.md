# 间距 Bug 深度复盘

> **编号**：RETRO-001
> **日期**：2026-05-24
> **问题**：Tailwind v4 全站 spacing utilities 静默失效，导致 UI 挤压
> **持续时间**：从 5 月 2 日项目创建到 5 月 24 日修复，共 **22 天**
> **修复提交**：`94cdc5b fix(css): remove manual * { padding: 0 } reset breaking Tailwind v4 utilities`
> **影响范围**：全站所有页面的 padding / margin / gap utilities
> **作者**：林夏 🎨

---

## 一、时间线

### 5 月 2 日 — 埋下隐患

提交 `7e4fcd8 feat(phase-0): 项目脚手架搭建完成`

在 `web/src/index.css` 中写入：

```css
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}
```

这是前端开发者的"肌肉记忆"——几乎所有项目都会加这段 reset。在 Tailwind v3 时代，这没有问题，因为 Tailwind v3 的 utilities 没有 `@layer` 包装，class 选择器优先级天然高于 `*` 通配符。

**但项目用的是 Tailwind v4，它彻底改变了这个前提。**

### 5 月 2 日 ~ 5 月 24 日 — 反复修间距，怎么改都没用

从那天起，UI 间距问题持续困扰，当天就进行了 **8 轮**修复尝试：

| 时间 | Commit | 描述 | 改动量 | 效果 |
|------|--------|------|--------|------|
| 16:05 | `ecfb679` | SF-FEAT0016 spacing/typography token | 8 文件 24 处 | ❌ 无效 |
| 18:05 | `cbc849b` | SF-FEAT0019 page-level style fixes | 7 文件 70 处 | ❌ 无效 |
| 20:43 | `f6c4bd5` | visual refresh — neutral palette | 1 文件 6 处 | ❌ 无效 |
| 21:29 | `7742005` | improve borders, spacing, readability | 5 文件 10 处 | ❌ 无效 |
| 23:06 | `59a81f3` | page padding, card containers | 6 文件 55 处 | ❌ 无效 |
| 23:26 | 4 分支并行 | 并行间距修复（global/dashboard/ticket-audit/users-settings） | 8 文件 38 处 | ❌ 无效 |
| 23:30 | `3e3dac5` | sidebar spacing 第 1 次 | 1 文件 7 处 | ❌ 无效 |
| 23:31 | `b6b333b` | sidebar spacing 第 2 次（加大幅度） | 1 文件 7 处 | ❌ 无效 |
| **23:40** | **`94cdc5b`** | **删除 `* { padding: 0 }`** | **1 文件 5 行** | **✅ 全站修复** |

**8 轮修复，30+ 文件，200+ 处修改，全部白费。最终只删了 5 行 CSS 就解决了。**

---

## 二、技术根因

### 2.1 Tailwind v4 的 `@layer` 架构变化

**Tailwind v3**（旧版）：

```css
/* 生成的 CSS 无 @layer 包装 */
.py-2\.5 { padding-top: 0.625rem; padding-bottom: 0.625rem; }
```

优先级：class 选择器 `(0,1,0)` > `*` 选择器 `(0,0,0)` → **utilities 赢** ✅

**Tailwind v4**（新版）：

```css
/* 生成的 CSS 被 @layer 包装 */
@layer base { *,::after,::before,::backdrop { margin: 0; padding: 0; } }
@layer utilities { .py-2\.5 { padding-block: calc(var(--spacing) * 2.5); } }
```

CSS `@layer` 优先级规则：

```
unlayered CSS  >  @layer utilities  >  @layer components  >  @layer base
```

用户的 `* { padding: 0 }` 在 `@layer` 之外（unlayered），所以它对 `.py-2\.5` 有**绝对优先权**，不管选择器特异性如何。

Tailwind v4 自己的 preflight reset 放在 `@layer base` 里，不会覆盖 `@layer utilities`，因为同一个 layering 系统内部按 specificity 正常排序。但 **unlayered CSS 对所有 layered CSS 有绝对优先权**。

### 2.2 为什么是"静默失效"？

- ✅ Tailwind class 正常出现在 HTML 中（DevTools Elements 面板能看到 `py-2.5`）
- ✅ CSS 规则也存在（DevTools Network 面板能看到 `.py-2\.5{...}`）
- ❌ 但 Computed Styles 显示 `padding-top: 0px`，被 `* { padding: 0 }` 覆盖
- ❌ **没有编译错误，没有运行时警告，没有任何报错**

这是一个"静默失效"的 bug——代码看起来完全正确，工具链没有报任何问题，但就是不生效。

---

## 三、为什么会反复踩坑？

### 3.1 "改了代码但没效果" → 怀疑自己的间距值

当开发者把 `py-2` 改成 `py-3`、`py-4`、甚至 `py-8`，刷新页面发现"没啥变化"时，自然的反应是：

1. **以为改得不够大** → 继续加大间距值
2. **以为浏览器缓存** → 清缓存、hard reload
3. **以为 Tailwind JIT 没扫描到** → 去检查 class name
4. **以为组件库覆盖了样式** → 去查 Shadcn/ui 源码

每一条都是合理的排查方向，但都指向错误的地方。因为真正的敌人是 CSS `@layer` 的优先级规则——一个 Tailwind v3 开发者不需要了解的知识点。

### 3.2 视觉问题难以量化

间距问题是"感觉不对"而非"功能坏了"。它不像 API 404 或 JS 报错那样有明确的错误信号。这导致：

- 开发者倾向于反复微调数值，而不是质疑底层机制
- Reviewer 看代码 diff 觉得"改得对"，但实际效果为零
- 每次修改后"好像好了一点"的错觉（确认偏误），实际是刷新页面后的主观感受波动

### 3.3 Tailwind v4 迁移缺乏感知

项目脚手架在 5/2 搭建时直接用了 Tailwind v4 + `@import "tailwindcss"`。开发者可能没有意识到这是 v4 的全新架构，仍然按 v3 的经验写 CSS。

---

## 四、本应在哪一步拦住？

### 第一道防线：改了不生效时，查 Computed Styles

第一次改间距没效果时，应该做一件事：

```
DevTools → Elements → 点击导航项 → Computed 面板 → 搜索 padding
```

会看到 `padding-top: 0px`，展开能看到覆盖源是 `* { padding: 0 }`。**5 分钟就能定位根因。**

### 第二道防线：用工具量化视觉问题

这次最终发现问题，是因为用了 Playwright 的 `getComputedStyle()` API 测量实际渲染值。当看到 `paddingTop: 0px` 时，问题立刻暴露了。

> **视觉 bug 应该用工具量化，而不是靠肉眼。**

### 第三道防线：Tailwind v4 知识更新

Tailwind v4 的 `@layer` 变化是 breaking change，但它是"静默的 breaking change"——迁移指南提到了 `@layer` 变化，但大多数开发者不会联想到"我手写的 `* { padding: 0 }` 会因此失效"。

### 第四道防线：Code Review

如果 Reviewer 在间距修复 PR 中问一句：

> "你改了 `py-2` → `py-4`，但在 DevTools Computed 里确认生效了吗？"

问题就能在第一轮被发现。

---

## 五、关键教训

### 1. 先验证假设，再大量修改

当第一次修改间距没效果时，应该**停下来验证**：

```
改了 py-2 → py-4 → 没效果
  → 打开 DevTools → 确认 class 在 HTML 中 ✅
  → 切到 Computed → 发现 padding = 0px ❌
  → 结论：CSS 规则被覆盖了，不是值的问题
  → 排查覆盖源 → 找到 * { padding: 0 }
  → 修复：删除手写 reset
```

而不是继续加大数值、清缓存、换浏览器。

### 2. 框架大版本升级要学习 breaking changes

Tailwind v3 → v4 的 `@layer` 变化是一个关键 breaking change。如果项目中混用了手写 CSS 和 Tailwind utilities，**必须了解 `@layer` 的优先级规则**。

### 3. "全局 reset" 是高风险代码

`* { margin: 0; padding: 0 }` 看似无害，但在 Tailwind v4 的 `@layer` 架构下，它会覆盖所有 spacing utilities。**任何全局选择器 `*` 的样式都应该放在 `@layer base` 内**，或者完全依赖框架提供的 reset。

### 4. 量化视觉问题

"间距不够"应该转化为"导航项 paddingTop 实际值是 0px，期望值是 10px"。量化后，问题立刻从"主观感受"变成"可调查的技术问题"。

### 5. 一次测量胜过十次修改

8 轮间距修复、30+ 文件、200+ 处修改、4 个并行分支……全部是在错误的假设下工作。如果第一轮就用 Playwright 测量一次实际渲染值，5 分钟就能定位根因。

---

## 六、行动项

| # | 行动 | 负责人 | 优先级 | 状态 |
|---|------|--------|--------|------|
| 1 | 团队分享：Tailwind v4 `@layer` 优先级规则与常见陷阱 | 林夏 | P1 | 待安排 |
| 2 | Code Review 清单增加：视觉/样式修改必须验证 Computed Styles | Marcus | P1 | 待更新 |
| 3 | 排查项目中其他可能的 unlayered CSS 覆盖 Tailwind utilities 的情况 | 林夏 | P2 | 待执行 |
| 4 | 更新脚手架模板：删除手写 reset，使用框架 preflight | Marcus | P2 | 待执行 |
| 5 | 前端开发规范增加：禁止在 `@layer` 之外编写全局 reset | 林夏 | P1 | 待写入 |

---

## 七、参考

- [CSS Cascade Layers (MDN)](https://developer.mozilla.org/en-US/docs/Web/CSS/@layer)
- [Tailwind CSS v4 Migration Guide — @layer](https://tailwindcss.com/docs/upgrade-guide#using-css-imports)
- [CSS Spec: Cascading and Inheritance Level 5](https://www.w3.org/TR/css-cascade-5/#layering)
- 修复提交：`94cdc5b`

---

*写下这些，是为了记住：当你的修改反复不生效时，停下来，用工具测量实际值，而不是继续在错误的方向上堆工作量。*
