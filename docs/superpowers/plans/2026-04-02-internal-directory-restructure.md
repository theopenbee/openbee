# internal/ 目录结构重构 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 `internal/` 下 24 个顶级包重组为按层分组的 7 个顶级目录（domain/、ai/、platform/、infra/、api/、app/、ctlclient/），同时合并碎片小包，提升代码可导航性和职责清晰度。

**Architecture:** 按照依赖顺序执行迁移：先移动无业务依赖的 infra 类包，再移动 ai 类包，最后移动 domain 类包；顶层的 api/、app/、ctlclient/ 位置不变，只更新其 import 路径。合并 task_scheduler+task_dispatcher→domain/task、claude+claudemd→ai/claude、media+ffmedia→infra/media、utils+toolnames→infra/utils。

**Tech Stack:** Go 1.25，模块名 `github.com/theopenbee/openbee`，使用 `mv` 移动文件，`sed -i` 批量替换 import 路径，`go build ./...` 逐步验证。

---

## 文件变更总览

### 新建目录
- `internal/domain/bee/`
- `internal/domain/worker/`
- `internal/domain/task/`
- `internal/domain/msgingest/`
- `internal/ai/claude/`
- `internal/ai/mcp/`
- `internal/infra/config/`
- `internal/infra/logger/`
- `internal/infra/auth/`
- `internal/infra/store/`
- `internal/infra/backup/`
- `internal/infra/i18n/`
- `internal/infra/model/`
- `internal/infra/utils/`
- `internal/infra/media/`
- `internal/infra/skillinstall/`

### 删除的目录（迁移完成后）
- `internal/task_scheduler/`
- `internal/task_dispatcher/`
- `internal/claudemd/`
- `internal/ffmedia/`
- `internal/toolnames/`
- 以及迁移到 infra/ 的所有原顶层目录

---

## Task 1: 迁移单文件 infra 包（config、logger、auth、backup、i18n、model、skillinstall）

**Files:**
- Move: `internal/config/` → `internal/infra/config/`
- Move: `internal/logger/` → `internal/infra/logger/`
- Move: `internal/auth/` → `internal/infra/auth/`
- Move: `internal/backup/` → `internal/infra/backup/`
- Move: `internal/i18n/` → `internal/infra/i18n/`
- Move: `internal/model/` → `internal/infra/model/`
- Move: `internal/skillinstall/` → `internal/infra/skillinstall/`

- [ ] **Step 1: 创建 infra 目录并移动包**

```bash
cd /path/to/openbee
mkdir -p internal/infra
mv internal/config internal/infra/config
mv internal/logger internal/infra/logger
mv internal/auth internal/infra/auth
mv internal/backup internal/infra/backup
mv internal/i18n internal/infra/i18n
mv internal/model internal/infra/model
mv internal/skillinstall internal/infra/skillinstall
```

- [ ] **Step 2: 批量替换 import 路径（全项目）**

```bash
# config
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/config"|"github.com/theopenbee/openbee/internal/infra/config"|g' {} +

# logger
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/logger"|"github.com/theopenbee/openbee/internal/infra/logger"|g' {} +

# auth
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/auth"|"github.com/theopenbee/openbee/internal/infra/auth"|g' {} +

# backup
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/backup"|"github.com/theopenbee/openbee/internal/infra/backup"|g' {} +

# i18n
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/i18n"|"github.com/theopenbee/openbee/internal/infra/i18n"|g' {} +

# model
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/model"|"github.com/theopenbee/openbee/internal/infra/model"|g' {} +

# skillinstall
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/skillinstall"|"github.com/theopenbee/openbee/internal/infra/skillinstall"|g' {} +
```

- [ ] **Step 3: 同样更新 store（store 依赖 model，需先确认 model 已更新）**

`internal/infra/store/` 的各文件中若有引用 model 包，已在上步更新。直接移动：

```bash
mv internal/store internal/infra/store
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/store"|"github.com/theopenbee/openbee/internal/infra/store"|g' {} +
```

- [ ] **Step 4: 验证编译**

```bash
go build ./...
```

预期：编译通过，无 import 错误。如有报错，检查是否有漏改的 import（`grep -r "internal/config\b" --include="*.go" .` 等）。

- [ ] **Step 5: 运行测试**

```bash
go test ./internal/infra/...
```

预期：所有测试 PASS。

- [ ] **Step 6: 提交**

```bash
git add -A
git commit -m "refactor: move infra packages to internal/infra/"
```

---

## Task 2: 合并 utils + toolnames → internal/infra/utils/

**Files:**
- Move: `internal/utils/*.go` → `internal/infra/utils/`
- Move: `internal/toolnames/toolnames.go` → `internal/infra/utils/toolnames.go`

- [ ] **Step 1: 移动 utils 并加入 toolnames**

```bash
mv internal/utils internal/infra/utils
mv internal/toolnames/toolnames.go internal/infra/utils/toolnames.go
rmdir internal/toolnames
```

- [ ] **Step 2: 确认 toolnames.go 的 package 声明**

打开 `internal/infra/utils/toolnames.go`，将 package 声明从：

```go
package toolnames
```

改为：

```go
package utils
```

- [ ] **Step 3: 批量替换 import 路径**

```bash
# utils
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/utils"|"github.com/theopenbee/openbee/internal/infra/utils"|g' {} +

# toolnames → utils（注意 toolnames. 调用方需改为 utils.）
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/toolnames"|"github.com/theopenbee/openbee/internal/infra/utils"|g' {} +
```

- [ ] **Step 4: 修复 toolnames 包名引用**

由于 `toolnames` 包已并入 `utils`，原来用 `toolnames.SomeConst` 的地方需改为 `utils.SomeConst`。找出所有调用方：

```bash
grep -rn "toolnames\." --include="*.go" .
```

对每个找到的文件，将 `toolnames.` 替换为 `utils.`：

```bash
find . -name "*.go" -exec sed -i '' 's/toolnames\./utils\./g' {} +
```

- [ ] **Step 5: 验证编译**

```bash
go build ./...
```

预期：编译通过。

- [ ] **Step 6: 运行测试**

```bash
go test ./internal/infra/utils/...
```

- [ ] **Step 7: 提交**

```bash
git add -A
git commit -m "refactor: merge utils+toolnames into internal/infra/utils"
```

---

## Task 3: 合并 media + ffmedia → internal/infra/media/

**Files:**
- Move: `internal/media/service.go` → `internal/infra/media/service.go`
- Move: `internal/ffmedia/ffmedia.go` → `internal/infra/media/ffmedia.go`

- [ ] **Step 1: 移动文件**

```bash
mv internal/media internal/infra/media
mv internal/ffmedia/ffmedia.go internal/infra/media/ffmedia.go
rmdir internal/ffmedia
```

- [ ] **Step 2: 修复 ffmedia.go 的 package 声明**

打开 `internal/infra/media/ffmedia.go`，将：

```go
package ffmedia
```

改为：

```go
package media
```

- [ ] **Step 3: 批量替换 import 路径**

```bash
# media
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/media"|"github.com/theopenbee/openbee/internal/infra/media"|g' {} +

# ffmedia → media
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/ffmedia"|"github.com/theopenbee/openbee/internal/infra/media"|g' {} +
```

- [ ] **Step 4: 修复 ffmedia 包名引用**

找出原来用 `ffmedia.` 的地方，改为 `media.`：

```bash
grep -rn "ffmedia\." --include="*.go" .
find . -name "*.go" -exec sed -i '' 's/ffmedia\./media\./g' {} +
```

- [ ] **Step 5: 验证编译**

```bash
go build ./...
```

- [ ] **Step 6: 运行测试**

```bash
go test ./internal/infra/media/...
```

- [ ] **Step 7: 提交**

```bash
git add -A
git commit -m "refactor: merge media+ffmedia into internal/infra/media"
```

---

## Task 4: 迁移 mcp → internal/ai/mcp/

**Files:**
- Move: `internal/mcp/` → `internal/ai/mcp/`

- [ ] **Step 1: 创建 ai 目录并移动 mcp**

```bash
mkdir -p internal/ai
mv internal/mcp internal/ai/mcp
```

- [ ] **Step 2: 批量替换 import 路径**

```bash
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/mcp"|"github.com/theopenbee/openbee/internal/ai/mcp"|g' {} +
```

- [ ] **Step 3: 验证编译**

```bash
go build ./...
```

- [ ] **Step 4: 运行测试**

```bash
go test ./internal/ai/mcp/...
```

- [ ] **Step 5: 提交**

```bash
git add -A
git commit -m "refactor: move mcp to internal/ai/mcp"
```

---

## Task 5: 合并 claude + claudemd → internal/ai/claude/

**Files:**
- Move: `internal/claude/*.go` → `internal/ai/claude/`
- Move: `internal/claudemd/claudemd.go` → `internal/ai/claude/claudemd.go`
- Move: `internal/claudemd/bee.go` → `internal/ai/claude/claudemd_bee.go`
- Move: `internal/claudemd/worker.go` → `internal/ai/claude/claudemd_worker.go`

注意：`claudemd` 的三个文件改名以避免与 claude 包中潜在的同名文件冲突（如 bee.go），同时表明其来源。

- [ ] **Step 1: 移动 claude 目录**

```bash
mv internal/claude internal/ai/claude
```

- [ ] **Step 2: 移动 claudemd 的文件，改包名**

```bash
mv internal/claudemd/claudemd.go internal/ai/claude/claudemd.go
mv internal/claudemd/bee.go internal/ai/claude/claudemd_bee.go
mv internal/claudemd/worker.go internal/ai/claude/claudemd_worker.go
rmdir internal/claudemd
```

- [ ] **Step 3: 修改 claudemd.go、claudemd_bee.go、claudemd_worker.go 的 package 声明**

将这三个文件顶部的 `package claudemd` 改为 `package claude`：

```bash
sed -i '' 's/^package claudemd$/package claude/' \
  internal/ai/claude/claudemd.go \
  internal/ai/claude/claudemd_bee.go \
  internal/ai/claude/claudemd_worker.go
```

- [ ] **Step 4: 批量替换 import 路径**

```bash
# claude
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/claude"|"github.com/theopenbee/openbee/internal/ai/claude"|g' {} +

# claudemd → ai/claude
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/claudemd"|"github.com/theopenbee/openbee/internal/ai/claude"|g' {} +
```

- [ ] **Step 5: 修复 claudemd 包名引用**

原来用 `claudemd.` 的地方现在应改为 `claude.`（因为合并后只有一个包名）：

```bash
grep -rn "claudemd\." --include="*.go" .
find . -name "*.go" -exec sed -i '' 's/claudemd\./claude\./g' {} +
```

注意：如果某个文件同时 import 了 `claude` 和 `claudemd`，合并后只需一个 import，删除重复的即可（go build 会提示 duplicate import）。

- [ ] **Step 6: 检查是否有重复 import**

```bash
go build ./...
```

如果报 `imported and not used` 或 `duplicate import`，打开对应文件删除多余的 import 行。

- [ ] **Step 7: 运行测试**

```bash
go test ./internal/ai/claude/...
```

- [ ] **Step 8: 提交**

```bash
git add -A
git commit -m "refactor: merge claude+claudemd into internal/ai/claude"
```

---

## Task 6: 合并 task_scheduler + task_dispatcher → internal/domain/task/

**Files:**
- Move: `internal/task_scheduler/scheduler.go` → `internal/domain/task/scheduler.go`
- Move: `internal/task_scheduler/scheduler_test.go` → `internal/domain/task/scheduler_test.go`
- Move: `internal/task_dispatcher/dispatcher.go` → `internal/domain/task/dispatcher.go`
- Move: `internal/task_dispatcher/dispatcher_test.go` → `internal/domain/task/dispatcher_test.go`
- Move: `internal/task_dispatcher/failure_notifier.go` → `internal/domain/task/failure_notifier.go`
- Move: `internal/task_dispatcher/failure_notifier_test.go` → `internal/domain/task/failure_notifier_test.go`
- Move: `internal/task_dispatcher/task.go` → `internal/domain/task/task.go`

注意：`task_scheduler/scheduler.go` 当前 import 了 `task_dispatcher`，合并后该 import 需要删除。

- [ ] **Step 1: 创建 domain 目录并移动文件**

```bash
mkdir -p internal/domain/task
mv internal/task_scheduler/scheduler.go internal/domain/task/scheduler.go
mv internal/task_scheduler/scheduler_test.go internal/domain/task/scheduler_test.go
mv internal/task_dispatcher/dispatcher.go internal/domain/task/dispatcher.go
mv internal/task_dispatcher/dispatcher_test.go internal/domain/task/dispatcher_test.go
mv internal/task_dispatcher/failure_notifier.go internal/domain/task/failure_notifier.go
mv internal/task_dispatcher/failure_notifier_test.go internal/domain/task/failure_notifier_test.go
mv internal/task_dispatcher/task.go internal/domain/task/task.go
rmdir internal/task_scheduler internal/task_dispatcher
```

- [ ] **Step 2: 修改所有移动文件的 package 声明**

```bash
# scheduler.go 和 scheduler_test.go：task_scheduler → task
sed -i '' 's/^package task_scheduler$/package task/' \
  internal/domain/task/scheduler.go \
  internal/domain/task/scheduler_test.go

# dispatcher.go 等：task_dispatcher → task
sed -i '' 's/^package task_dispatcher$/package task/' \
  internal/domain/task/dispatcher.go \
  internal/domain/task/dispatcher_test.go \
  internal/domain/task/failure_notifier.go \
  internal/domain/task/failure_notifier_test.go \
  internal/domain/task/task.go
```

- [ ] **Step 3: 删除 scheduler.go 中对 task_dispatcher 的 import**

打开 `internal/domain/task/scheduler.go`，找到并删除这行 import（因为 task_dispatcher 已经和 scheduler 在同一包内）：

```go
"github.com/theopenbee/openbee/internal/task_dispatcher"
```

同时将文件中所有 `task_dispatcher.DispatchTask` 改为 `DispatchTask`（去掉包前缀）：

```bash
sed -i '' 's/task_dispatcher\.DispatchTask/DispatchTask/g' internal/domain/task/scheduler.go
sed -i '' 's/task_dispatcher\.DispatchTask/DispatchTask/g' internal/domain/task/scheduler_test.go
```

然后手动打开 `scheduler.go` 确认 `task_dispatcher` import 行已删除（或用 `goimports` 自动清理）：

```bash
# 如果安装了 goimports
goimports -w internal/domain/task/scheduler.go
```

若未安装 goimports，手动编辑删除该 import 行即可。

- [ ] **Step 4: 批量替换全项目 import 路径**

```bash
# task_scheduler
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/task_scheduler"|"github.com/theopenbee/openbee/internal/domain/task"|g' {} +

# task_dispatcher
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/task_dispatcher"|"github.com/theopenbee/openbee/internal/domain/task"|g' {} +
```

- [ ] **Step 5: 修复包名引用（task_scheduler. 和 task_dispatcher. → task.）**

```bash
grep -rn "task_scheduler\.\|task_dispatcher\." --include="*.go" .
find . -name "*.go" -exec sed -i '' 's/task_scheduler\./task\./g' {} +
find . -name "*.go" -exec sed -i '' 's/task_dispatcher\./task\./g' {} +
```

注意：`app.go` 同时使用了 `task_scheduler.Scheduler` 和 `task_dispatcher.TaskDispatcher`、`task_dispatcher.DispatchTask` 等类型，替换后都变成 `task.XXX`，需确认无重名冲突（同包内类型名唯一即可）。

- [ ] **Step 6: 检查重复 import**

由于 `app.go` 原来同时 import 了 `task_scheduler` 和 `task_dispatcher`，替换后会出现两个指向同一路径的 import，需要手动删除重复项：

打开 `internal/app/app.go`，找到类似：
```go
"github.com/theopenbee/openbee/internal/domain/task"
"github.com/theopenbee/openbee/internal/domain/task"
```
删除其中一行。worker/manager.go 等文件同理。

- [ ] **Step 7: 验证编译**

```bash
go build ./...
```

- [ ] **Step 8: 运行测试**

```bash
go test ./internal/domain/task/...
```

- [ ] **Step 9: 提交**

```bash
git add -A
git commit -m "refactor: merge task_scheduler+task_dispatcher into internal/domain/task"
```

---

## Task 7: 迁移剩余 domain 包（bee、worker、msgingest）

**Files:**
- Move: `internal/bee/` → `internal/domain/bee/`
- Move: `internal/worker/` → `internal/domain/worker/`
- Move: `internal/msgingest/` → `internal/domain/msgingest/`

- [ ] **Step 1: 移动包**

```bash
mv internal/bee internal/domain/bee
mv internal/worker internal/domain/worker
mv internal/msgingest internal/domain/msgingest
```

- [ ] **Step 2: 批量替换 import 路径**

```bash
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/bee"|"github.com/theopenbee/openbee/internal/domain/bee"|g' {} +
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/worker"|"github.com/theopenbee/openbee/internal/domain/worker"|g' {} +
find . -name "*.go" -exec sed -i '' 's|"github.com/theopenbee/openbee/internal/msgingest"|"github.com/theopenbee/openbee/internal/domain/msgingest"|g' {} +
```

- [ ] **Step 3: 验证编译**

```bash
go build ./...
```

预期：编译通过。此时所有包均已迁移到新位置。

- [ ] **Step 4: 确认旧顶层目录已全部清空**

```bash
ls internal/
```

预期输出（只有 7 个条目）：
```
ai/
api/
app/
ctlclient/
domain/
infra/
platform/
```

若有残留目录，检查是否漏移（`find internal -maxdepth 1 -type d` 查看）。

- [ ] **Step 5: 运行所有测试**

```bash
go test ./...
```

预期：所有测试 PASS。

- [ ] **Step 6: 提交**

```bash
git add -A
git commit -m "refactor: move bee, worker, msgingest to internal/domain/"
```

---

## Task 8: 全量验证

- [ ] **Step 1: 完整编译**

```bash
go build ./...
```

预期：0 错误，0 警告。

- [ ] **Step 2: 完整测试**

```bash
go test ./...
```

预期：所有测试 PASS，无 FAIL。

- [ ] **Step 3: 确认无残留旧 import**

```bash
grep -rn "internal/task_scheduler\|internal/task_dispatcher\|internal/claudemd\|internal/ffmedia\|internal/toolnames" --include="*.go" .
```

预期：无输出（0 匹配）。

```bash
grep -rn '"github.com/theopenbee/openbee/internal/config"' --include="*.go" .
```

预期：无输出（应已全部替换为 infra/config）。

- [ ] **Step 4: 最终提交**

```bash
git add -A
git commit -m "refactor: complete internal/ directory restructure to domain/ai/infra layers"
```
