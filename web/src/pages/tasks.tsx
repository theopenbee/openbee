import { useState } from "react"
import { useTranslation } from "react-i18next"
import { useTasks } from "@/hooks/use-tasks"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { TaskList, TASK_PAGE_SIZE } from "@/components/task-list"

export function Tasks() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const { data } = useTasks({ page, pageSize: TASK_PAGE_SIZE })
  const totalTasks = data?.total ?? 0

  return (
    <FadeIn>
      <PageHeader
        title={t("tasks.title")}
        subtitle={totalTasks > 0 ? t("tasks.summary", { count: totalTasks }) : undefined}
      />
      <TaskList page={page} pageSize={TASK_PAGE_SIZE} onPageChange={setPage} />
    </FadeIn>
  )
}
