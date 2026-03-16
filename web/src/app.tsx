import { HashRouter, Routes, Route } from "react-router-dom"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { Layout } from "@/components/layout"
import { Dashboard } from "@/pages/dashboard"
import { Workers } from "@/pages/workers"
import { WorkerDetail } from "@/pages/worker-detail"
import { Executions } from "@/pages/executions"
import { ExecutionDetail } from "@/pages/execution-detail"
import { SessionDetail } from "@/pages/session-detail"

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
    },
  },
})

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <HashRouter>
        <Routes>
          <Route element={<Layout />}>
            <Route path="/" element={<Dashboard />} />
            <Route path="/workers" element={<Workers />} />
            <Route path="/workers/:id" element={<WorkerDetail />} />
            <Route path="/executions" element={<Executions />} />
            <Route path="/executions/:id" element={<ExecutionDetail />} />
            <Route path="/sessions/:sessionId" element={<SessionDetail />} />
          </Route>
        </Routes>
      </HashRouter>
    </QueryClientProvider>
  )
}
