# Worker 部门功能设计

## 概述

为 Worker 新增部门（Department）功能，支持多层级部门结构，Worker 可以属于多个部门。部门的主要用途是 **UI 展示与组织管理**，方便用户对 Worker 进行分组查找和管理。

## 设计决策

| 决策项 | 选择 | 原因 |
|--------|------|------|
| 用途 | 展示与组织 | 纯 UI 分组，不涉及任务路由或权限隔离 |
| 层级结构 | 无限层级 | parent_id 自引用树，灵活性高，实现复杂度差异不大 |
| 归属关系 | 平等归属 | 多对多，无主次之分，关联表实现 |
| 部门属性 | 最简 | id、name、parent_id、sort_order、时间戳 |
| 删除策略 | 仅允许删除空部门 | 最安全，避免误操作 |
| API 返回格式 | 树形结构 | 后端组装树形 JSON，前端直接渲染 |
| 前端 UI | 左侧部门树 + 右侧 Worker 列表 | 树形导航最直观 |
| 数据存储方案 | 独立表 + 关联表 | 标准关系型设计，查询灵活，数据完整性好 |

## 数据模型

### 新增表：bee_departments

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT PRIMARY KEY | UUID |
| name | TEXT NOT NULL | 部门名称 |
| parent_id | TEXT | 父部门 ID，NULL 表示顶级部门 |
| sort_order | INTEGER DEFAULT 0 | 同级排序，数值越小越靠前 |
| created_at | INTEGER NOT NULL | 创建时间（Unix 毫秒） |
| updated_at | INTEGER NOT NULL | 更新时间（Unix 毫秒） |

索引：
- `idx_departments_parent_id ON bee_departments(parent_id)`

### 新增表：bee_worker_departments

| 字段 | 类型 | 说明 |
|------|------|------|
| worker_id | TEXT NOT NULL | Worker ID |
| department_id | TEXT NOT NULL | 部门 ID |
| created_at | INTEGER NOT NULL | 关联创建时间（Unix 毫秒） |

约束：
- PRIMARY KEY (worker_id, department_id) — 联合主键，防止重复关联
- worker_id 外键关联 bee_workers(id)
- department_id 外键关联 bee_departments(id)

索引：
- `idx_worker_depts_worker ON bee_worker_departments(worker_id)`
- `idx_worker_depts_dept ON bee_worker_departments(department_id)`

### Go Model 定义

```go
// internal/infra/model/department.go

type Department struct {
    ID        string  `json:"id" db:"id"`
    Name      string  `json:"name" db:"name"`
    ParentID  *string `json:"parent_id" db:"parent_id"`
    SortOrder int     `json:"sort_order" db:"sort_order"`
    CreatedAt int64   `json:"created_at" db:"created_at"`
    UpdatedAt int64   `json:"updated_at" db:"updated_at"`
}

type DepartmentTree struct {
    Department
    Children []DepartmentTree `json:"children"`
}

type WorkerDepartment struct {
    WorkerID     string `json:"worker_id" db:"worker_id"`
    DepartmentID string `json:"department_id" db:"department_id"`
    CreatedAt    int64  `json:"created_at" db:"created_at"`
}
```

## API 设计

### 部门 CRUD

#### POST /api/departments — 创建部门

```json
// Request
{"name": "技术部", "parent_id": null, "sort_order": 0}

// Response 201
{"id": "uuid", "name": "技术部", "parent_id": null, "sort_order": 0, "created_at": 1712300000000, "updated_at": 1712300000000}
```

校验规则：
- name 必填，不能为空
- parent_id 如果提供，必须是已存在的部门 ID

#### GET /api/departments — 获取部门树

```json
// Response 200
[
  {
    "id": "1", "name": "技术部", "parent_id": null, "sort_order": 0,
    "created_at": 1712300000000, "updated_at": 1712300000000,
    "children": [
      {
        "id": "2", "name": "后端组", "parent_id": "1", "sort_order": 0,
        "created_at": 1712300000000, "updated_at": 1712300000000,
        "children": []
      }
    ]
  }
]
```

后端从数据库获取所有部门（平铺），在内存中组装为树形结构返回。部门数据量通常很小，无性能问题。

#### GET /api/departments/:id — 获取单个部门

```json
// Response 200
{"id": "1", "name": "技术部", "parent_id": null, "sort_order": 0, "created_at": 1712300000000, "updated_at": 1712300000000}
```

#### PUT /api/departments/:id — 更新部门

```json
// Request（部分更新，字段可选）
{"name": "技术中心", "parent_id": "new-parent-id", "sort_order": 1}

// Response 200 — 返回更新后的部门
```

校验规则：
- 更新 parent_id 时需校验不能形成循环引用
- 循环检测方法：从目标 parent 开始向上遍历祖先链，如果遇到当前部门 ID，说明会形成环，拒绝操作

#### DELETE /api/departments/:id — 删除部门

```json
// 成功 Response 200
{"status": "deleted"}

// 失败（有子部门或关联 Worker）Response 400
{"error": "department is not empty: has 2 sub-departments and 3 workers"}
```

删除前检查：
1. 该部门下是否有子部门
2. 该部门是否有关联的 Worker
3. 两者都没有才允许删除

### Worker-部门关联

#### PUT /api/workers/:id/departments — 设置 Worker 的部门（全量替换）

```json
// Request
{"department_ids": ["dept-1", "dept-2"]}

// Response 200
{"department_ids": ["dept-1", "dept-2"]}
```

逻辑：在事务中删除该 Worker 的所有旧关联，插入新关联。传入空数组 `[]` 表示清空所有部门归属。

#### GET /api/workers/:id/departments — 获取 Worker 所属部门列表

```json
// Response 200
[
  {"id": "dept-1", "name": "技术部", "parent_id": null, "sort_order": 0, "created_at": ..., "updated_at": ...},
  {"id": "dept-2", "name": "翻译部", "parent_id": null, "sort_order": 1, "created_at": ..., "updated_at": ...}
]
```

#### GET /api/departments/:id/workers — 获取某部门下的 Worker 列表

```json
// Response 200
[
  {"id": "w1", "name": "毛毛", "status": "idle", ...},
  {"id": "w2", "name": "小蜜", "status": "working", ...}
]
```

只返回直接关联到该部门的 Worker，不递归子部门。

### 对现有 API 的改动

**GET /api/workers** 和 **GET /api/workers/:id** 的返回中增加 `departments` 字段：

```json
{
  "id": "w1", "name": "毛毛", "status": "idle",
  "departments": [{"id": "dept-1", "name": "技术部"}, {"id": "dept-2", "name": "翻译部"}],
  ...
}
```

**GET /api/workers** 增加可选查询参数 `?department_id=xxx` 用于按部门筛选。

## 后端代码结构

### 新增文件

| 文件 | 说明 |
|------|------|
| `internal/infra/model/department.go` | Department、DepartmentTree、WorkerDepartment 模型 |
| `internal/infra/store/department_store.go` | 部门 CRUD + 树形查询 + Worker 关联操作 |
| `internal/api/department_handler.go` | 部门相关 HTTP handler |

### 修改文件

| 文件 | 改动 |
|------|------|
| `internal/infra/store/db.go` | 新增 2 张表的 migration |
| `internal/api/router.go` | 注册部门相关路由 |
| `internal/api/worker_handler.go` | listWorkers/getWorker 返回中附带 departments |
| `internal/app/app.go` | 初始化 DepartmentStore 并注入 |

### DepartmentStore 核心方法

```go
type DepartmentStore struct { db *sql.DB }

// 部门 CRUD
func (s *DepartmentStore) Create(d model.Department) (model.Department, error)
func (s *DepartmentStore) GetByID(id string) (model.Department, error)
func (s *DepartmentStore) Update(d model.Department) (model.Department, error)
func (s *DepartmentStore) Delete(id string) error

// 树形查询
func (s *DepartmentStore) ListAll() ([]model.Department, error)
func (s *DepartmentStore) BuildTree(depts []model.Department) []model.DepartmentTree

// Worker-部门关联
func (s *DepartmentStore) SetWorkerDepartments(workerID string, deptIDs []string) error
func (s *DepartmentStore) GetWorkerDepartments(workerID string) ([]model.Department, error)
func (s *DepartmentStore) GetDepartmentWorkers(deptID string) ([]model.Worker, error)
func (s *DepartmentStore) HasChildren(deptID string) (bool, error)
func (s *DepartmentStore) HasWorkers(deptID string) (bool, error)
```

### 循环引用检测

更新 parent_id 时，从目标 parent 开始向上遍历祖先链，如果遇到当前部门 ID 则拒绝操作。部门数据量小，遍历代价可忽略。

## 前端设计

### 新增/修改文件

| 文件 | 说明 |
|------|------|
| `web/src/hooks/use-departments.ts` | 部门相关 React Query hooks |
| `web/src/lib/api.ts` | 新增 api.departments.* 方法 |
| `web/src/components/department-tree.tsx` | 部门树形组件（左侧导航用） |
| `web/src/pages/workers.tsx` | 改造为左侧部门树 + 右侧列表布局 |
| `web/src/components/department-dialog.tsx` | 创建/编辑部门对话框 |
| `web/src/components/worker-department-select.tsx` | Worker 编辑时的多选部门组件 |

### 交互流程

1. **Workers 页面**改造为左右布局：
   - 左侧：部门树（可折叠/展开），顶部有「全部 Worker」节点和「未分组」节点
   - 右侧：Worker 列表（根据选中的部门节点过滤）
   - 左侧底部：「管理部门」按钮，打开部门管理对话框

2. **部门管理对话框**：
   - 树形展示所有部门
   - 支持：新增部门（选择父部门）、重命名、删除（空部门）、拖拽排序

3. **Worker 编辑**：
   - 在 Worker 详情页增加「所属部门」字段
   - 使用多选下拉组件（带树形层级展示），可选择多个部门

4. **特殊节点**：
   - 「全部 Worker」：显示所有 Worker，不区分部门
   - 「未分组」：显示未关联任何部门的 Worker

## 错误处理

| 场景 | 处理方式 |
|------|---------|
| 创建部门时 parent_id 不存在 | 返回 400，提示 parent department not found |
| 更新 parent_id 导致循环引用 | 返回 400，提示 circular reference detected |
| 删除非空部门 | 返回 400，提示 department is not empty 并说明有多少子部门和关联 Worker |
| 设置 Worker 部门时 department_id 不存在 | 返回 400，提示 department not found: {id} |
| 删除 Worker 时 | 自动清理 bee_worker_departments 中该 Worker 的所有关联记录 |

## 测试策略

- DepartmentStore 单元测试：CRUD、树形组装、关联操作、循环引用检测、空部门删除校验
- API 集成测试：各端点的正常和异常路径
- 前端组件测试：部门树渲染、筛选交互
