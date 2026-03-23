import { lazy, Suspense } from "react"
import { HashRouter, Routes, Route } from "react-router-dom"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { Layout } from "@/components/layout"
import { AuthGuard } from "@/components/auth-guard"

const Login = lazy(() => import("@/pages/login").then(m => ({ default: m.Login })))
const Dashboard = lazy(() => import("@/pages/dashboard").then(m => ({ default: m.Dashboard })))
const Workers = lazy(() => import("@/pages/workers").then(m => ({ default: m.Workers })))
const WorkerDetail = lazy(() => import("@/pages/worker-detail").then(m => ({ default: m.WorkerDetail })))
const Executions = lazy(() => import("@/pages/executions").then(m => ({ default: m.Executions })))
const ExecutionDetail = lazy(() => import("@/pages/execution-detail").then(m => ({ default: m.ExecutionDetail })))
const SessionDetail = lazy(() => import("@/pages/session-detail").then(m => ({ default: m.SessionDetail })))
const LocalChat = lazy(() => import("@/pages/local-chat").then(m => ({ default: m.LocalChat })))
const LocalChatDetail = lazy(() => import("@/pages/local-chat-detail").then(m => ({ default: m.LocalChatDetail })))

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
        <Suspense fallback={null}>
          <Routes>
            <Route path="/login" element={<Login />} />
            <Route element={<AuthGuard><Layout /></AuthGuard>}>
              <Route path="/" element={<Dashboard />} />
              <Route path="/workers" element={<Workers />} />
              <Route path="/workers/:id" element={<WorkerDetail />} />
              <Route path="/executions" element={<Executions />} />
              <Route path="/executions/:id" element={<ExecutionDetail />} />
              <Route path="/sessions/:sessionId" element={<SessionDetail />} />
              <Route path="/local-chat" element={<LocalChat />} />
              <Route path="/local-chat/:id" element={<LocalChatDetail />} />
            </Route>
          </Routes>
        </Suspense>
      </HashRouter>
    </QueryClientProvider>
  )
}
