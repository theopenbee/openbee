# OpenBee Web `/login` Redesign

Date: 2026-04-05
Status: Approved for planning
Owner: Codex

## Summary

重设计 OpenBee Web 的 [`/login`](../../../../web/src/pages/login.tsx) 页面，使其从通用卡片式登录页升级为带有明确品牌识别的控制台入口页。

最终方向采用：

- 品牌定位：`品牌入口`
- 核心意象：`蜂巢中枢`
- 首要信息：`这是你的蜂巢控制入口`
- 页面结构：`双区结构`
- 视觉方向：`Signal Split`

页面需要在不牺牲登录效率的前提下，建立“进入蜂巢控制中枢”的第一印象，并与 OpenBee 当前的控制台产品气质保持一致。

## Goals

- 让 `/login` 一眼可识别为 OpenBee，而不是通用后台模板。
- 保持登录流程单线程、低认知负担，输入和提交动作必须始终清晰。
- 将“蜂巢”表达为中枢拓扑、节点、信号网络，而不是卡通蜂窝或营销插画。
- 在桌面端建立强识别的品牌入口感，在移动端保留同一叙事但压缩信息密度。
- 复用现有 UI 基础设施、认证逻辑、国际化和主题切换机制。

## Non-Goals

- 不修改登录 API、token 存储、跳转逻辑或认证协议。
- 不引入“忘记密码”“注册”“SSO”“记住我”等新流程。
- 不把登录页做成营销主页、产品介绍页或仪表板预览页。
- 不重建一套脱离现有站点的视觉系统。

## Design Context

当前产品的已知设计上下文如下：

- 用户：在本机或服务器上运行 AI workers 的运维和技术团队。
- 场景：进入 Web 控制台以管理 workers、任务、会话、执行记录和排障流程。
- 品牌气质：直接、可靠、操作型、安静自信。
- 当前站点原则：高扫描效率、状态识别优先、结构化数据优先、轻量动效、响应式密度控制。

登录页需要在这套上下文内成为“入口页”，而不是偏离产品的单独品牌秀场。

## Experience Narrative

用户进入 `/login` 后，应感受到自己正在进入一个正在运作的蜂巢控制系统。

这个页面不做“欢迎来到 OpenBee”的泛化营销表达，而是明确传达：

- 这里是蜂巢控制入口。
- 你需要完成身份校验后回到 worker、任务和执行中枢。
- 界面具备品牌性，但依旧是操作型工具，而不是宣传画面。

最终体验应更接近“进入控制系统前的认证闸门”，而不是“企业官网的登录页”。

## Information Architecture

页面采用双区布局。

### Left Pane: Hive Signal Stage

左侧用于建立品牌和场景认知，承担以下职责：

- 以大标题建立“蜂巢中枢入口”的主叙事。
- 用节点、连线、核心发光区、少量状态语义构建中枢拓扑感。
- 用一组简短辅助信息连接到后续产品语义，如 worker、activity、control、queue。
- 强化这是控制台入口，而非一般表单页。

左侧不是营销文案区，不放长段介绍，不放 feature list，不放按钮组。

### Right Pane: Authentication Panel

右侧是唯一的主任务区，承担以下职责：

- 显示登录标题和一句简短说明。
- 提供用户名和密码输入框。
- 显示错误提示和提交按钮。
- 保留轻量辅助元素，例如实例身份提示或简短支持信息。

右侧需要视觉对比更高、边界更清晰，使用户进入页面后的最终视线稳定落在表单区。

## Layout Specification

### Desktop

- 页面占满视口高度。
- 顶部保留轻量 header，包含 OpenBee 品牌标识和主题切换。
- 主体为左右双区：
  - 左侧约 `58% - 62%`
  - 右侧约 `38% - 42%`
- 左侧包含：
  - eyebrow/kicker
  - 主标题
  - 一句控制台式说明文案
  - 中枢拓扑图形区域
  - 3 个简短信号信息块
- 右侧包含：
  - 认证 kicker
  - 登录标题
  - 简短提示
  - 用户名输入
  - 密码输入
  - 固定位置错误提示区
  - 提交按钮
  - 底部辅助信息

### Mobile

- 改为上下结构，不做双栏硬压缩。
- 品牌 header 保留，但更紧凑。
- 左侧内容收缩为顶部品牌区：
  - 标题行数减少
  - 文案缩短
  - 拓扑图压缩为较矮的横向品牌带
- 登录表单完整保留在下方。
- 不隐藏核心认证功能，不让图形区在视觉上压过表单。

## Visual Direction

### Tone

- 冷静、精密、控制台式。
- 不是极简白页，也不是高饱和未来风。
- 不是卡通蜂巢，不是拟物插画，不是营销渐变。

### Color

建议使用：

- 基础底色：石墨黑、煤灰、带少量暖色倾向的深色中性
- 强调色：蜂蜜金 / amber 信号色
- 辅助文字：暖灰蓝调中性字色

约束：

- 黄色只用于信号点亮处、主按钮、微小重点。
- 不使用满屏大面积金色或高饱和橙黄。
- 不使用青蓝霓虹、紫蓝渐变、荧光描边等典型 AI 风格。

### Shape Language

- 蜂巢概念通过节点、连线、核心区、网格、六边形语义弱化表达。
- 不直接堆叠大面积蜂窝格。
- 圆角应偏克制，不走玩具化圆润风格。
- 阴影应轻，质感主要来自分层、边界和局部光感。

### Typography

- 延续项目现有偏技术品牌的字体方向。
- 标题要更紧凑、更有压缩感，避免松散的 SaaS 风格。
- 正文保持高可读性，不追求装饰性字体秀。
- 文案整体简短，避免重复用户已经看得见的信息。

## Content Strategy

页面文案采用“系统入口提示”语气，而不是品牌广告语气。

### Recommended Copy Shape

- 左侧大标题：直接说明“蜂巢正在等待指令”或“进入控制中枢”这一类入口感表达。
- 左侧说明：一句，强调登录后回到 worker、任务、执行等控制流程。
- 右侧标题：明确是进入 Hive Control 或 Sign In。
- 错误提示：沿用现有登录错误语义，不写情绪化文案。

### Copy Constraints

- 不写长段品牌故事。
- 不罗列 feature bullets。
- 不重复“Sign In”含义。
- 中英文语言包都要覆盖新增支持文案。

## Interaction Design

### Initial Load

- 页面进入时可使用一次轻量分层进入动画。
- 左右区域可存在轻微错峰显现，但总时长必须短。
- 如果用户设置减少动态效果，应弱化或关闭非必要动效。

### Focus and Input

- 用户名输入框自动聚焦。
- 输入顺序固定为用户名 -> 密码 -> 提交。
- 表单区不引入额外分支动作。

### Submit State

- 提交中时按钮进入明确 loading 状态。
- 用户名和密码输入框在提交中禁用。
- 防止重复提交。

### Error Handling

- 错误提示固定显示在按钮上方。
- 为错误提示预留稳定空间，避免布局跳动。
- `401`、`429`、通用错误都映射到同一错误区块。

### Theme Toggle

- 顶部保留轻量主题切换。
- 主题切换应服务现有 light/dark 机制，不单独为登录页发明新方案。

## Accessibility

- 保证标题、正文、输入框、按钮和错误提示有足够对比度。
- 错误提示应被辅助技术正确感知。
- 焦点状态必须清晰可见，不被装饰层吞掉。
- 页面在键盘操作下可完整完成登录。
- 动画需考虑 `prefers-reduced-motion`。

## Implementation Design

### Existing Files

- [`web/src/pages/login.tsx`](../../../../web/src/pages/login.tsx)
- [`web/src/globals.css`](../../../../web/src/globals.css)
- [`web/src/locales/en.json`](../../../../web/src/locales/en.json)
- [`web/src/locales/zh.json`](../../../../web/src/locales/zh.json)
- [`web/src/components/theme-switcher.tsx`](../../../../web/src/components/theme-switcher.tsx)

### Proposed Structure

推荐将登录页拆成页面内局部结构，而不是继续用单个 `Card` 直接包表单。

可接受的实现方式：

- 在 [`web/src/pages/login.tsx`](../../../../web/src/pages/login.tsx) 中直接定义页面结构，前提是结构仍然清晰。
- 如果 JSX 变复杂，可在同目录或 `components/` 中抽出纯展示型局部子组件，例如：
  - `LoginHero`
  - `LoginSignalGraph`
  - `LoginAuthPanel`

是否拆分为独立组件，以可读性为准，不为抽象而抽象。

### Data Flow

- 继续复用现有 `login(username, password)`。
- 继续复用现有 `navigate("/", { replace: true })` 成功跳转。
- 保留现有错误状态分支：
  - `401`
  - `429`
  - generic failure
- 不改 token 存储逻辑。

### Styling Approach

- 优先使用现有 Tailwind / shadcn / 全局 token 体系。
- 如需要登录页专属视觉细节，可在全局样式中添加少量可复用 utility 或 token。
- 避免为单页引入过多一次性全局污染。

### Internationalization

- 保留现有字段：
  - `login.title`
  - `login.username`
  - `login.password`
  - `login.submit`
  - `login.error401`
  - `login.error429`
  - `login.errorGeneric`
- 为新页面新增必要文案键，例如：
  - left pane kicker
  - hero title
  - hero description
  - auth panel helper text
  - instance/support hint

新增文案必须同时补齐 `en.json` 和 `zh.json`。

## Motion

- 只保留 1 次主要入场动效。
- 图形层可有极轻微呼吸或信号感，但必须低频、低幅度。
- 不使用 bounce、elastic 或炫技型持续动画。
- 动效服务层级感，不服务“炫”。

## Testing Strategy

### Functional

- 用户名、密码输入和提交仍可正常工作。
- 成功登录后正确跳转。
- `401`、`429`、网络失败的反馈正确显示。
- 提交中状态会禁用重复提交。

### Responsive

- 桌面宽屏下双区结构正常。
- 平板宽度下两区比例不会挤压表单可用性。
- 手机宽度下切换为上下结构，表单优先级正确。

### Accessibility

- 键盘可完成登录。
- 焦点可见。
- 错误信息可读。
- 明暗主题下对比达标。

### Visual Regression Watchpoints

- 左侧图形区不能喧宾夺主。
- 右侧输入与按钮必须始终是最易识别的交互对象。
- 登录页不能看起来像官网着陆页或管理后台首页。
- 移动端不能出现图形区过高导致表单首屏不可见的问题。

## Acceptance Criteria

- `/login` 不再是单一居中卡片布局。
- 页面具备明确的 OpenBee 品牌识别和“蜂巢中枢入口”气质。
- 登录动作依旧是页面最明确的主任务。
- 现有认证逻辑与错误处理行为保持正确。
- 明暗主题和中英文语言均正常工作。
- 响应式布局在手机上可用且不丢失品牌叙事。

## Open Questions Resolved

- 是否做传统双栏品牌页：是，但左侧为控制中枢叙事，不是营销介绍。
- 是否强化蜂巢感：是，但以拓扑/节点/信号表达，不做直白蜂窝插画。
- 是否保留高效登录：是，右侧始终保持单线程认证面板。
- 是否增加额外认证流程：否。
