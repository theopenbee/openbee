import { useState } from "react"
import { useTranslation } from "react-i18next"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { TaskList, TASK_PAGE_SIZE } from "@/components/task-list"

export function Tasks() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)

  return (
    <FadeIn>
      <PageHeader title={t("tasks.title")} />
      <TaskList page={page} pageSize={TASK_PAGE_SIZE} onPageChange={setPage} />
    </FadeIn>
  )
}
