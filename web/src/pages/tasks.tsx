import { useTranslation } from "react-i18next"
import { PageHeader } from "@/components/page-header"
import { FadeIn } from "@/components/fade-in"
import { TaskList } from "@/components/task-list"

export function Tasks() {
  const { t } = useTranslation()
  return (
    <FadeIn>
      <PageHeader title={t("tasks.title")} />
      <TaskList />
    </FadeIn>
  )
}
