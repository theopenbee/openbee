import { lazy, Suspense } from "react"
import { HashRouter, Routes, Route } from "react-router-dom"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { Layout } from "@/components/layout"
import { AuthGuard } from "@/components/auth-guard"
import { TooltipProvider } from "@/components/ui/tooltip"

const Login = lazy(() => import("@/pages/login").then(m => ({ default: m.Login })))
const Dashboard = lazy(() => import("@/pages/dashboard").then(m => ({ default: m.Dashboard })))
const Workers = lazy(() => import("@/pages/workers").then(m => ({ default: m.Workers })))
const WorkerDetail = lazy(() => import("@/pages/worker-detail").then(m => ({ default: m.WorkerDetail })))
const Executions = lazy(() => import("@/pages/executions").then(m => ({ default: m.Executions })))
const SessionDetail = lazy(() => import("@/pages/session-detail").then(m => ({ default: m.SessionDetail })))
const LocalChat = lazy(() => import("@/pages/local-chat").then(m => ({ default: m.LocalChat })))
const Tasks = lazy(() => import("@/pages/tasks").then(m => ({ default: m.Tasks })))
const Departments = lazy(() => import("@/pages/departments").then(m => ({ default: m.Departments })))
const Settings = lazy(() => import("@/pages/settings").then(m => ({ default: m.Settings })))

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
      <TooltipProvider>
        <HashRouter>
          <Suspense fallback={null}>
            <Routes>
              <Route path="/login" element={<Login />} />
              <Route element={<AuthGuard><Layout /></AuthGuard>}>
                <Route path="/" element={<Dashboard />} />
                <Route path="/workers" element={<Workers />} />
                <Route path="/workers/:id" element={<WorkerDetail />} />
                <Route path="/departments" element={<Departments />} />
                <Route path="/sessions" element={<Executions />} />
                <Route path="/sessions/detail" element={<SessionDetail />} />
                <Route path="/tasks" element={<Tasks />} />
                <Route path="/chat" element={<LocalChat />} />
                <Route path="/env" element={<Settings />} />
              </Route>
            </Routes>
          </Suspense>
        </HashRouter>
      </TooltipProvider>
    </QueryClientProvider>
  )
}
