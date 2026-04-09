# /chat 页面 @ 功能 UI 设计文档

**日期：** 2026-04-09
**状态：** 已确认，待实现

## 背景

后端已实现 `@姓名 ` 直接调度（PR #70）：消息以 `@name ` 开头时，Feeder 直接将任务路由给对应 Worker，跳过 Bee。

本文档定义前端 `/chat` 页面输入框的 @ 触发 UI，让用户在输入时可通过弹出面板选择 Worker，选中后自动插入 `@姓名 `。

## 决策摘要

| 问题 | 决策 |
|---|---|
| Worker 面板显示内容 | 只显示名字，不显示状态 |
| `@` 触发位置 | 任意位置均可触发 |
| 无匹配时行为 | 自动关闭面板 |
| 实现方案 | 方案二：提取 `MentionTextarea` 组件 |
| 匹配策略 | `startsWith`（大小写不敏感） |

## 文件变更

```
web/src/components/mention-textarea.tsx   ← 新增
web/src/pages/local-chat.tsx              ← 改动（替换 textarea，传入 workers）
```

## 组件接口

### `MentionTextarea` props

```tsx
interface MentionTextareaProps {
  value: string
  onChange: (value: string) => void
  onKeyDown?: (e: React.KeyboardEvent<HTMLTextAreaElement>) => void
  onPaste?: (e: React.ClipboardEvent<HTMLTextAreaElement>) => void
  workers: Array<{ id: string; name: string }>
  placeholder?: string
  disabled?: boolean
  textareaRef?: React.RefObject<HTMLTextAreaElement>
  className?: string
}
```

- `value / onChange`：受控组件模式，与父组件 `input` state 直接对接
- `workers`：由父组件传入，`MentionTextarea` 不自行发请求
- `onKeyDown`：父组件传入（用于 Enter 发送），面板激活时组件优先消耗键盘事件，未消耗时转发
- `textareaRef`：父组件传入，保持现有 focus/scroll 逻辑不变
- `disabled`：`isProcessing=true` 时传入，面板同步关闭

## 内部状态

```tsx
type MentionState = {
  query: string        // @ 后已输入的搜索词（空字符串 = 刚触发）
  triggerIndex: number // @ 字符在 value 中的位置索引
  activeIndex: number  // 键盘当前高亮的候选项索引
}

const [mentionState, setMentionState] = useState<MentionState | null>(null)
```

`null` 表示面板关闭。

## 核心逻辑

### @ 检测（detectMention）

```ts
function detectMention(value: string, caretPos: number): MentionState | null {
  const textBefore = value.slice(0, caretPos)
  const atIndex = textBefore.lastIndexOf("@")
  if (atIndex === -1) return null

  const fragment = textBefore.slice(atIndex + 1)
  // @ 到光标之间有空格或换行，说明已完结
  if (fragment.includes(" ") || fragment.includes("\n")) return null

  return { query: fragment, triggerIndex: atIndex, activeIndex: 0 }
}
```

### onChange 处理

```ts
function handleChange(e: React.ChangeEvent<HTMLTextAreaElement>) {
  const newValue = e.target.value
  onChange(newValue)

  const caret = e.target.selectionStart ?? newValue.length
  const state = detectMention(newValue, caret)

  if (state) {
    const matched = workers.filter(w =>
      w.name.toLowerCase().startsWith(state.query.toLowerCase())
    )
    setMentionState(matched.length > 0 ? state : null)
  } else {
    setMentionState(null)
  }
}
```

### 键盘交互（onKeyDown）

面板激活时：
- `ArrowDown`：高亮下移，阻止默认滚动
- `ArrowUp`：高亮上移，阻止默认滚动
- `Enter`：选中当前高亮 Worker，**阻止消息发送**
- `Escape`：关闭面板，保留已输入文字

面板未激活时：所有键盘事件转发给父组件 `onKeyDown`。

### 选中逻辑（handleSelect）

```ts
function handleSelect(worker: { id: string; name: string }) {
  if (!mentionState) return
  const textarea = textareaRef?.current
  const caret = textarea?.selectionStart ?? value.length

  const before = value.slice(0, mentionState.triggerIndex)
  const after = value.slice(caret)
  const inserted = `@${worker.name} `

  onChange(before + inserted + after)
  setMentionState(null)

  // 光标移到插入内容末尾
  requestAnimationFrame(() => {
    if (textarea) {
      const pos = mentionState.triggerIndex + inserted.length
      textarea.setSelectionRange(pos, pos)
      textarea.focus()
    }
  })
}
```

### 面板关闭（onBlur）

```ts
function handleBlur() {
  // 延迟关闭，给候选项的 onMouseDown 留执行窗口
  setTimeout(() => setMentionState(null), 150)
}
```

## 面板 UI

面板定位在输入框**上方**，与输入框等宽，最多显示 8 条候选（超出可滚动）。

`filteredWorkers` 在渲染时推导：

```ts
const filteredWorkers = mentionState
  ? workers
      .filter(w => w.name.toLowerCase().startsWith(mentionState.query.toLowerCase()))
      .slice(0, 8)
  : []
```

```tsx
<div className="relative">
  {mentionState && filteredWorkers.length > 0 && (
    <div className="absolute bottom-full left-0 right-0 mb-1 z-50
                    rounded-2xl border border-border/70 bg-popover
                    shadow-lg overflow-hidden">
      <ul role="listbox" className="max-h-[280px] overflow-y-auto py-1">
        {filteredWorkers.map((worker, index) => (
          <li
            key={worker.id}
            role="option"
            aria-selected={index === mentionState.activeIndex}
            className={cn(
              "flex items-center px-4 py-2.5 text-sm cursor-pointer transition-colors",
              index === mentionState.activeIndex
                ? "bg-accent text-accent-foreground"
                : "hover:bg-accent/50"
            )}
            onMouseDown={(e) => {
              e.preventDefault() // 保持 textarea 焦点
              handleSelect(worker)
            }}
          >
            <span className="font-medium truncate">{worker.name}</span>
          </li>
        ))}
      </ul>
    </div>
  )}
  <textarea ... />
</div>
```

**`onMouseDown` + `e.preventDefault()` 关键：** 避免 textarea 在 `onClick` 前 blur 导致面板关闭、选中事件丢失。

## local-chat.tsx 改动

```tsx
// 新增 import
import { useWorkers } from "@/hooks/use-workers"
import { MentionTextarea } from "@/components/mention-textarea"

// 组件内新增
const { data: workers } = useWorkers()
const workerList = (workers ?? []).map(w => ({ id: w.id, name: w.name }))

// 替换原 <textarea>（约 15 行变动）
<MentionTextarea
  textareaRef={textareaRef}
  value={input}
  onChange={setInput}
  onKeyDown={(e) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault()
      void handleSend()
    }
  }}
  onPaste={handlePaste}
  workers={workerList}
  placeholder={t("localChat.inputPlaceholder")}
  disabled={isProcessing}
  className="max-h-[220px] min-h-[120px] w-full resize-none bg-transparent px-3 py-2 text-sm leading-7 placeholder:text-muted-foreground focus:outline-none"
/>
```

现有 `useEffect` 自动调整高度逻辑（监听 `input`，设置 `textarea.style.height`）保持不变。

## 边界情况

| 情况 | 处理方式 |
|---|---|
| Worker 列表为空 | `filteredWorkers` 为空，面板不显示，`@` 作为普通文本 |
| `@` 后无匹配 | `filteredWorkers` 为空，立即关闭面板 |
| 消息中多个 `@` | 只响应光标前最近的有效 `@`，前一个 `@name ` 已含空格不会重激活 |
| `isProcessing=true` | `disabled` 传入，textarea 禁用，面板因 blur 关闭 |
| Enter 键冲突 | 面板激活时 Enter 选中并 `preventDefault`，阻止发送 |
| 删除 `@` 字符 | `detectMention` 返回 null，面板关闭 |
| 手动在 `@name` 后加空格 | fragment 含空格，面板关闭 |
| 粘贴图片 | `onPaste` 由父组件处理，行为不变 |
| Worker 名超长 | 面板内 `truncate` 截断 |
| 候选超过 8 条 | 列表内部滚动，`max-h-[280px] overflow-y-auto` |

## 不在本次范围内

- 消息气泡中 `@姓名` 的高亮渲染（仅影响已发送消息展示）
- Task 创建输入框的 @ 功能（未来可复用 `MentionTextarea`）
- `@` 在消息中间时的后端感知提示（Bee 会理解上下文）
