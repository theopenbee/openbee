# Worker Random Name Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a shuffle button to the worker creation form that fills the name field with a random unused name from a built-in pool of 200 characters.

**Architecture:** Backend exposes `GET /api/workers/random-name` (JWT-protected); the handler reads `cfg.Language` to choose either the Three Kingdoms Chinese pool or the historical scientists English pool, queries existing worker names, filters out used ones, and returns a random pick or an exhausted flag. The frontend shuffle button calls this endpoint and updates the name input.

**Tech Stack:** Go 1.21 + Gin (backend), React + TanStack Query + i18next (frontend), SQLite (store), lucide-react icons, shadcn/ui Tooltip.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `internal/domain/worker/names.go` | Create | Name pool constants + `namePool()` + `pickRandomName()` |
| `internal/domain/worker/names_test.go` | Create | Tests for `pickRandomName` |
| `internal/infra/store/worker_store.go` | Modify | Add `ListNames()` method |
| `internal/infra/store/worker_store_test.go` | Modify | Add `TestWorkerStore_ListNames` |
| `internal/api/worker_handler.go` | Modify | Add `language` field + `RandomName` handler |
| `internal/app/app.go` | Modify | Pass `language` to `NewWorkerHandler` |
| `internal/routes/api.go` | Modify | Register `GET /workers/random-name` |
| `web/src/lib/api.ts` | Modify | Add `workers.randomName()` |
| `web/src/hooks/use-workers.ts` | Modify | Add `useRandomWorkerName()` hook |
| `web/src/locales/en.json` | Modify | Add 2 i18n keys |
| `web/src/locales/zh.json` | Modify | Add 2 i18n keys |
| `web/src/components/create-worker-sheet.tsx` | Modify | Add shuffle button to name field |

---

## Task 1: Name Pool Data and Pick Function

**Files:**
- Create: `internal/domain/worker/names.go`
- Create: `internal/domain/worker/names_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/domain/worker/names_test.go`:

```go
package worker

import (
	"testing"
)

func TestPickRandomName_AllUnused(t *testing.T) {
	pool := []string{"Alice", "Bob", "Carol"}
	used := map[string]struct{}{}
	name, ok := PickRandomName(pool, used)
	if !ok {
		t.Fatal("expected ok=true, got false")
	}
	found := false
	for _, p := range pool {
		if p == name {
			found = true
		}
	}
	if !found {
		t.Errorf("returned name %q not in pool", name)
	}
}

func TestPickRandomName_SomeUsed(t *testing.T) {
	pool := []string{"Alice", "Bob", "Carol"}
	used := map[string]struct{}{"alice": {}, "bob": {}}
	name, ok := PickRandomName(pool, used)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if name != "Carol" {
		t.Errorf("expected Carol, got %q", name)
	}
}

func TestPickRandomName_AllUsed(t *testing.T) {
	pool := []string{"Alice", "Bob"}
	used := map[string]struct{}{"alice": {}, "bob": {}}
	_, ok := PickRandomName(pool, used)
	if ok {
		t.Fatal("expected ok=false when all names used")
	}
}

func TestPickRandomName_CaseInsensitive(t *testing.T) {
	pool := []string{"Alice"}
	used := map[string]struct{}{"ALICE": {}}
	_, ok := PickRandomName(pool, used)
	if ok {
		t.Fatal("expected ok=false: pool name should be filtered case-insensitively")
	}
}

func TestNamePool_ZH(t *testing.T) {
	pool := NamePool("zh")
	if len(pool) != 200 {
		t.Errorf("zh pool: want 200 names, got %d", len(pool))
	}
}

func TestNamePool_EN(t *testing.T) {
	pool := NamePool("en")
	if len(pool) != 200 {
		t.Errorf("en pool: want 200 names, got %d", len(pool))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/domain/worker/... -run TestPickRandom -v
```

Expected: `FAIL — undefined: PickRandomName`

- [ ] **Step 3: Create `internal/domain/worker/names.go`**

```go
package worker

import (
	"math/rand"
	"strings"
)

var zhNames = []string{
	"曹操", "刘备", "孙权", "诸葛亮", "关羽", "张飞", "赵云", "马超", "黄忠", "魏延",
	"姜维", "邓艾", "钟会", "司马懿", "司马昭", "司马师", "夏侯惇", "夏侯渊", "曹仁", "曹洪",
	"曹纯", "曹休", "曹真", "张辽", "乐进", "于禁", "徐晃", "张郃", "许褚", "典韦",
	"荀彧", "荀攸", "贾诩", "郭嘉", "程昱", "刘晔", "蒋济", "华歆", "王朗", "董昭",
	"满宠", "毛玠", "崔琰", "陈群", "司马朗", "钟繇", "王粲", "杨修", "孔融", "陈琳",
	"曹丕", "曹植", "曹彰", "曹冲", "许攸", "审配", "逢纪", "郭图", "田丰", "沮授",
	"颜良", "文丑", "高览", "袁绍", "袁术", "袁谭", "袁尚", "袁熙", "刘表", "蔡瑁",
	"蒯越", "刘琦", "刘琮", "黄祖", "甘宁", "凌统", "周瑜", "鲁肃", "吕蒙", "陆逊",
	"陆抗", "程普", "黄盖", "韩当", "蒋钦", "周泰", "陈武", "潘璋", "丁奉", "徐盛",
	"朱然", "孙策", "孙坚", "孙翊", "孙皎", "孙亮", "孙休", "孙皓", "张昭", "张纮",
	"顾雍", "诸葛瑾", "步骘", "吾粲", "骆统", "虞翻", "陆绩", "张温", "严畯", "薛综",
	"程秉", "周鲂", "是仪", "吕范", "朱桓", "全琮", "吕岱", "孙韶", "诸葛恪", "孙峻",
	"孙綝", "濮阳兴", "张布", "万彧", "刘禅", "关兴", "关平", "张苞", "马谡", "王平",
	"廖化", "向宠", "蒋琬", "费祎", "董允", "陈祗", "吕乂", "张翼", "柳隐", "罗宪",
	"霍峻", "霍弋", "邓芝", "樊建", "宗预", "辅匡", "孙干", "简雍", "麋竺", "麋芳",
	"貂蝉", "王允", "吕布", "陈宫", "高顺", "宋宪", "魏续", "侯成", "张邈", "陈登",
	"陶谦", "曹嵩", "丁原", "何进", "董卓", "李傕", "郭汜", "皇甫嵩", "朱儁", "卢植",
	"刘虞", "公孙瓒", "张燕", "张绣", "张角", "张宝", "张梁", "管亥", "波才", "程远志",
	"华雄", "吕公", "纪灵", "桥玄", "何颙", "蒯良", "张肃", "刘繇", "许劭", "郑玄",
	"蔡邕", "王越", "史阿", "胡轸", "吕旷", "吕翔", "眭固", "韩浩", "史涣", "韩遂",
}

var enNames = []string{
	"Newton", "Darwin", "Einstein", "Tesla", "Curie", "Faraday", "Galileo", "Copernicus", "Kepler", "Brahe",
	"Maxwell", "Planck", "Bohr", "Heisenberg", "Schrodinger", "Feynman", "Dirac", "Rutherford", "Thomson", "Chadwick",
	"Mendeleev", "Lavoisier", "Priestley", "Dalton", "Avogadro", "Boltzmann", "Carnot", "Joule", "Kelvin", "Rankine",
	"Euler", "Gauss", "Riemann", "Cauchy", "Fourier", "Laplace", "Lagrange", "Pascal", "Fermat", "Archimedes",
	"Pythagoras", "Euclid", "Eratosthenes", "Hipparchus", "Ptolemy", "Thales", "Democritus", "Aristotle", "Bacon", "Descartes",
	"Hooke", "Boyle", "Huygens", "Columbus", "Vespucci", "Magellan", "Drake", "Cook", "Cabot", "Polo",
	"Mendel", "Lamarck", "Linnaeus", "Buffon", "Cuvier", "Huxley", "Haeckel", "Pasteur", "Koch", "Lister",
	"Fleming", "Jenner", "Harvey", "Vesalius", "Hippocrates", "Watt", "Stephenson", "Edison", "Marconi", "Bell",
	"Morse", "Babbage", "Lovelace", "Turing", "Hubble", "Sagan", "Hawking", "Penrose", "Dyson", "Oppenheimer",
	"Fermi", "Compton", "Ampere", "Volta", "Ohm", "Coulomb", "Hertz", "Lorentz", "Mach", "Doppler",
	"Celsius", "Fahrenheit", "Clausius", "Helmholtz", "Kirchhoff", "Bunsen", "Liebig", "Kekule", "Herschel", "Cassini",
	"Halley", "Flamsteed", "Bradley", "Bessel", "Fraunhofer", "Huggins", "Adams", "Leverrier", "Roentgen", "Becquerel",
	"Meitner", "Hahn", "Szilard", "Teller", "Bethe", "Seaborg", "Nobel", "Benz", "Wright", "Goddard",
	"Braun", "Glenn", "Armstrong", "Watson", "Crick", "Franklin", "Wilkins", "McClintock", "Morgan", "Muller",
	"Beadle", "Tatum", "Avery", "Wegener", "Richter", "Lyell", "Hutton", "Agassiz", "Holmes", "Wilson",
	"Shannon", "Wiener", "Neumann", "Hopper", "Knuth", "Dijkstra", "Chomsky", "McCarthy", "Minsky", "Wirth",
	"Vinci", "Gutenberg", "Wren", "Brunel", "Newcomen", "Langley", "Lilienthal", "Zeppelin", "Wallace", "Henslow",
	"Gamow", "Milne", "Chandrasekhar", "Pauli", "Born", "Sommerfeld", "Moseley", "Soddy", "Aston", "Lemaitre",
	"Langmuir", "Millikan", "Michelson", "Morley", "Raman", "Bose", "Ramanujan", "Hardy", "Littlewood", "Eddington",
	"Poincare", "Hilbert", "Cantor", "Dedekind", "Peano", "Frege", "Russell", "Whitehead", "Godel", "Church",
}

func NamePool(lang string) []string {
	if lang == "zh" {
		return zhNames
	}
	return enNames
}

func PickRandomName(pool []string, used map[string]struct{}) (string, bool) {
	available := make([]string, 0, len(pool))
	for _, name := range pool {
		if _, ok := used[strings.ToLower(name)]; !ok {
			available = append(available, name)
		}
	}
	if len(available) == 0 {
		return "", false
	}
	return available[rand.Intn(len(available))], true
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/domain/worker/... -run "TestPickRandom|TestNamePool" -v
```

Expected: all 6 tests PASS

- [ ] **Step 5: Commit**

```bash
git add internal/domain/worker/names.go internal/domain/worker/names_test.go
git commit -m "feat: add worker name pool and pickRandomName"
```

---

## Task 2: WorkerStore.ListNames()

**Files:**
- Modify: `internal/infra/store/worker_store.go`
- Modify: `internal/infra/store/worker_store_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/infra/store/worker_store_test.go`:

```go
func TestWorkerStore_ListNames(t *testing.T) {
	s := setupTestDB(t)

	// Empty store returns empty slice without error
	names, err := s.ListNames()
	if err != nil {
		t.Fatalf("ListNames on empty store: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 names, got %d", len(names))
	}

	// Seed two workers
	s.Create(model.Worker{Name: "Alpha"})
	s.Create(model.Worker{Name: "Beta"})

	names, err = s.ListNames()
	if err != nil {
		t.Fatalf("ListNames: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d: %v", len(names), names)
	}

	nameSet := map[string]bool{}
	for _, n := range names {
		nameSet[n] = true
	}
	if !nameSet["Alpha"] || !nameSet["Beta"] {
		t.Errorf("expected Alpha and Beta in %v", names)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/infra/store/... -run TestWorkerStore_ListNames -v
```

Expected: `FAIL — s.ListNames undefined`

- [ ] **Step 3: Add ListNames to worker_store.go**

Append to `internal/infra/store/worker_store.go`, after the `List()` method:

```go
func (s *WorkerStore) ListNames() ([]string, error) {
	rows, err := s.db.Query(`SELECT name FROM bee_workers`)
	if err != nil {
		return nil, fmt.Errorf("list worker names: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/infra/store/... -run TestWorkerStore_ListNames -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/infra/store/worker_store.go internal/infra/store/worker_store_test.go
git commit -m "feat: add WorkerStore.ListNames for random name generation"
```

---

## Task 3: RandomName Handler + Route

**Files:**
- Modify: `internal/api/worker_handler.go`
- Modify: `internal/app/app.go`
- Modify: `internal/routes/api.go`

- [ ] **Step 1: Update WorkerHandler struct and constructor**

In `internal/api/worker_handler.go`, replace:

```go
type WorkerHandler struct {
	workers     *store.WorkerStore
	departments *store.DepartmentStore
	manager     *worker.Manager
}

func NewWorkerHandler(ws *store.WorkerStore, ds *store.DepartmentStore, mgr *worker.Manager) *WorkerHandler {
	return &WorkerHandler{workers: ws, departments: ds, manager: mgr}
}
```

with:

```go
type WorkerHandler struct {
	workers     *store.WorkerStore
	departments *store.DepartmentStore
	manager     *worker.Manager
	language    string
}

func NewWorkerHandler(ws *store.WorkerStore, ds *store.DepartmentStore, mgr *worker.Manager, lang string) *WorkerHandler {
	return &WorkerHandler{workers: ws, departments: ds, manager: mgr, language: lang}
}
```

- [ ] **Step 2: Add strings import and RandomName method**

Add `"strings"` to the import block in `internal/api/worker_handler.go`:

```go
import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/theopenbee/openbee/internal/domain/worker"
	"github.com/theopenbee/openbee/internal/infra/auth"
	"github.com/theopenbee/openbee/internal/infra/model"
	"github.com/theopenbee/openbee/internal/infra/store"
)
```

Append the following method to `internal/api/worker_handler.go`:

```go
func (h *WorkerHandler) RandomName(c *gin.Context) {
	pool := worker.NamePool(h.language)
	names, err := h.workers.ListNames()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	used := make(map[string]struct{}, len(names))
	for _, n := range names {
		used[strings.ToLower(n)] = struct{}{}
	}
	name, ok := worker.PickRandomName(pool, used)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"exhausted": true})
		return
	}
	c.JSON(http.StatusOK, gin.H{"name": name})
}
```

The handler calls `worker.NamePool` and `worker.PickRandomName` — both are exported as defined in Task 1.

- [ ] **Step 3: Pass language to NewWorkerHandler in app.go**

In `internal/app/app.go`, find line 331 and change:

```go
Workers: api.NewWorkerHandler(s.workerStore, s.departmentStore, mgr),
```

to:

```go
Workers: api.NewWorkerHandler(s.workerStore, s.departmentStore, mgr, language),
```

- [ ] **Step 4: Register the route in routes/api.go**

In `internal/routes/api.go`, add this line **before** `r.GET("/workers/:id", ...)`:

```go
r.GET("/workers/random-name", s.Workers.RandomName)
```

The full worker routes block should look like:

```go
r.POST("/workers", s.Workers.Create)
r.GET("/workers", s.Workers.List)
r.GET("/workers/random-name", s.Workers.RandomName)
r.GET("/workers/:id", s.Workers.Get)
r.PUT("/workers/:id", s.Workers.Update)
r.DELETE("/workers/:id", s.Workers.Delete)
```

- [ ] **Step 5: Build to verify no compilation errors**

```bash
go build ./...
```

Expected: exits 0, no errors

- [ ] **Step 6: Commit**

```bash
git add internal/api/worker_handler.go internal/app/app.go internal/routes/api.go internal/domain/worker/names.go internal/domain/worker/names_test.go
git commit -m "feat: add GET /workers/random-name endpoint"
```

---

## Task 4: Frontend API Method and Hook

**Files:**
- Modify: `web/src/lib/api.ts`
- Modify: `web/src/hooks/use-workers.ts`

- [ ] **Step 1: Add randomName to api.ts**

In `web/src/lib/api.ts`, inside the `workers` object (after the `executions` method, before the closing `}`):

```ts
randomName: () => fetchAPI<{ name?: string; exhausted?: boolean }>("/workers/random-name"),
```

The full `workers` object ends with:

```ts
    executions: async (id: string, page: number = 1, pageSize: number = 20) => {
      return fetchAPI<PaginatedResponse<WorkerExecution>>(
        `/workers/${id}/executions?page=${page}&page_size=${pageSize}`
      )
    },
    randomName: () => fetchAPI<{ name?: string; exhausted?: boolean }>("/workers/random-name"),
  },
```

- [ ] **Step 2: Add useRandomWorkerName hook**

Append to `web/src/hooks/use-workers.ts`:

```ts
export function useRandomWorkerName() {
  return useMutation({
    mutationFn: () => api.workers.randomName(),
  })
}
```

- [ ] **Step 3: Verify TypeScript compilation**

```bash
cd web && npx tsc --noEmit
```

Expected: exits 0

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/api.ts web/src/hooks/use-workers.ts
git commit -m "feat: add randomName API method and useRandomWorkerName hook"
```

---

## Task 5: i18n Keys

**Files:**
- Modify: `web/src/locales/en.json`
- Modify: `web/src/locales/zh.json`

- [ ] **Step 1: Add keys to en.json**

In `web/src/locales/en.json`, inside `workers.form`, after `"nameHelper": "Used for identification"`:

```json
"randomName": "Random name",
"randomNameExhausted": "All names are in use",
```

- [ ] **Step 2: Add keys to zh.json**

In `web/src/locales/zh.json`, inside `workers.form`, after `"nameHelper": "用于身份识别"`:

```json
"randomName": "随机姓名",
"randomNameExhausted": "所有名字已被使用",
```

- [ ] **Step 3: Commit**

```bash
git add web/src/locales/en.json web/src/locales/zh.json
git commit -m "feat: add i18n keys for random name feature"
```

---

## Task 6: Shuffle Button UI

**Files:**
- Modify: `web/src/components/create-worker-sheet.tsx`

- [ ] **Step 1: Add imports**

In `create-worker-sheet.tsx`, update the import block:

```tsx
import { useState, useEffect, useRef, type FormEvent } from "react"
import { useTranslation } from "react-i18next"
import { ChevronDown, Search, Shuffle, Loader2 } from "lucide-react"
import { useCreateWorker, useRandomWorkerName } from "@/hooks/use-workers"
import { useFlatDepartments, useSetWorkerDepartments } from "@/hooks/use-departments"
import { useEnabledEngines } from "@/hooks/use-config"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Separator } from "@/components/ui/separator"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetFooter,
} from "@/components/ui/sheet"
import {
  Select,
  SelectContent,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { EngineSelectItems } from "@/components/engine-select-items"
import { SectionHeading } from "@/components/section-heading"
import { KNOWN_SCOPES, serializeScopes, parseScopes, toggleScope } from "@/lib/scopes"
import { cn, getErrorMessage } from "@/lib/utils"
import type { Worker, Engine } from "@/lib/types"
import { DEFAULT_ENGINE, pickDefaultEngine } from "@/lib/types"
```

- [ ] **Step 2: Add state and hook inside CreateWorkerSheet**

Inside `CreateWorkerSheet`, after `const [deptSearch, setDeptSearch] = useState("")`:

```tsx
const [nameExhausted, setNameExhausted] = useState(false)
const randomName = useRandomWorkerName()
```

- [ ] **Step 3: Reset nameExhausted when sheet opens**

Inside the `useEffect` that runs when `open` changes, add after `setDeptSearch("")`:

```tsx
setNameExhausted(false)
```

- [ ] **Step 4: Add handleRandomName handler**

Inside `CreateWorkerSheet`, after the `handleSubmit` function:

```tsx
const handleRandomName = async () => {
  const result = await randomName.mutateAsync()
  if (result.exhausted) {
    setNameExhausted(true)
  } else if (result.name) {
    setName(result.name)
  }
}
```

- [ ] **Step 5: Replace the name Input with the flex row containing the shuffle button**

Find this block in the JSX (lines ~149–162):

```tsx
<div className="space-y-1.5">
  <Label htmlFor="cws-name">
    {t("workers.form.name")}
    <span className="ml-1 text-destructive" aria-hidden>*</span>
  </Label>
  <Input
    id="cws-name"
    value={name}
    onChange={(e) => setName(e.target.value)}
    placeholder={t("workers.form.namePlaceholder")}
    required
    autoFocus
  />
  <p className="text-xs text-muted-foreground">{t("workers.form.nameHelper")}</p>
</div>
```

Replace it with:

```tsx
<div className="space-y-1.5">
  <Label htmlFor="cws-name">
    {t("workers.form.name")}
    <span className="ml-1 text-destructive" aria-hidden>*</span>
  </Label>
  <div className="flex gap-2">
    <Input
      id="cws-name"
      value={name}
      onChange={(e) => setName(e.target.value)}
      placeholder={t("workers.form.namePlaceholder")}
      required
      autoFocus
      className="flex-1"
    />
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            variant="outline"
            size="icon"
            disabled={nameExhausted || randomName.isPending}
            onClick={handleRandomName}
            aria-label={t("workers.form.randomName")}
          >
            {randomName.isPending
              ? <Loader2 className="size-4 animate-spin" />
              : <Shuffle className="size-4" />
            }
          </Button>
        </TooltipTrigger>
        {nameExhausted && (
          <TooltipContent>
            <p>{t("workers.form.randomNameExhausted")}</p>
          </TooltipContent>
        )}
      </Tooltip>
    </TooltipProvider>
  </div>
  <p className="text-xs text-muted-foreground">{t("workers.form.nameHelper")}</p>
</div>
```

- [ ] **Step 6: Verify TypeScript compilation**

```bash
cd web && npx tsc --noEmit
```

Expected: exits 0

- [ ] **Step 7: Start dev server and manually test**

```bash
cd web && npm run dev
```

Open the workers page, click "Create Worker". Verify:
1. Shuffle button (🔀) appears to the right of the name input
2. Clicking it fills the name field with a random character name
3. Clicking again fills a different name
4. Button shows a spinner while loading
5. Copy-worker flow still pre-fills name with original + suffix

- [ ] **Step 8: Commit**

```bash
git add web/src/components/create-worker-sheet.tsx
git commit -m "feat: add random name shuffle button to worker creation form"
```
