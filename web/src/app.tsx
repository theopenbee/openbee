import { lazy, Suspense } from "react"
import { HashRouter, Routes, Route } from "react-router-dom"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { Toaster } from "sonner"
import { Layout } from "@/components/layout"
import { AuthGuard } from "@/components/auth-guard"
import { Guard } from "@/components/guard"
import { Home } from "@/pages/home"
import { Perm } from "@/lib/permissions"
import { isForbidden } from "@/lib/api"
import { TooltipProvider } from "@/components/ui/tooltip"

const Login = lazy(() => import("@/pages/login").then(m => ({ default: m.Login })))
const Setup = lazy(() => import("@/pages/setup").then(m => ({ default: m.Setup })))
const Dashboard = lazy(() => import("@/pages/dashboard").then(m => ({ default: m.Dashboard })))
const Workers = lazy(() => import("@/pages/workers").then(m => ({ default: m.Workers })))
const CreateWorker = lazy(() => import("@/pages/create-worker").then(m => ({ default: m.CreateWorker })))
const WorkerDetail = lazy(() => import("@/pages/worker-detail").then(m => ({ default: m.WorkerDetail })))
const Sessions = lazy(() => import("@/pages/sessions").then(m => ({ default: m.Sessions })))
const SessionDetail = lazy(() => import("@/pages/session-detail").then(m => ({ default: m.SessionDetail })))
const LocalChat = lazy(() => import("@/pages/local-chat").then(m => ({ default: m.LocalChat })))
const Tasks = lazy(() => import("@/pages/tasks").then(m => ({ default: m.Tasks })))
const Departments = lazy(() => import("@/pages/departments").then(m => ({ default: m.Departments })))
const Users = lazy(() => import("@/pages/users").then(m => ({ default: m.Users })))
const Roles = lazy(() => import("@/pages/roles").then(m => ({ default: m.Roles })))
const Env = lazy(() => import("@/pages/env").then(m => ({ default: m.Settings })))
const SystemSettings = lazy(() => import("@/pages/settings").then(m => ({ default: m.SystemSettings })))
const NotFound = lazy(() => import("@/pages/not-found").then(m => ({ default: m.NotFound })))

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
      // Surface a 403 to the nearest ForbiddenBoundary so it renders the shared
      // no-permission state; other errors stay in component state as before.
      throwOnError: (error) => isForbidden(error),
    },
  },
})

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <Toaster theme="system" richColors />
      <TooltipProvider>
        <HashRouter>
          <Suspense fallback={null}>
            <Routes>
              <Route path="/login" element={<Login />} />
              <Route path="/setup" element={<Setup />} />
              <Route element={<AuthGuard><Layout /></AuthGuard>}>
                <Route path="/" element={<Home><Dashboard /></Home>} />
                <Route path="/workers" element={<Guard perm={Perm.WorkersRead}><Workers /></Guard>} />
                <Route path="/workers/create" element={<Guard perm={Perm.WorkersWrite}><CreateWorker /></Guard>} />
                <Route path="/workers/:id" element={<Guard perm={Perm.WorkersRead}><WorkerDetail /></Guard>} />
                <Route path="/departments" element={<Guard perm={Perm.DepartmentsRead}><Departments /></Guard>} />
                <Route path="/users" element={<Guard perm={Perm.UsersManage}><Users /></Guard>} />
                <Route path="/roles" element={<Guard perm={Perm.RolesManage}><Roles /></Guard>} />
                <Route path="/sessions" element={<Guard perm={Perm.SessionsRead}><Sessions /></Guard>} />
                <Route path="/sessions/detail" element={<Guard perm={Perm.SessionsRead}><SessionDetail /></Guard>} />
                <Route path="/tasks" element={<Guard perm={Perm.TasksRead}><Tasks /></Guard>} />
                <Route path="/chat" element={<LocalChat />} />
                <Route path="/env" element={<Guard perm={Perm.EnvRead}><Env /></Guard>} />
                <Route path="/settings" element={<Guard perm={Perm.SystemConfigRead}><SystemSettings /></Guard>} />
              </Route>
              <Route path="*" element={<NotFound />} />
            </Routes>
          </Suspense>
        </HashRouter>
      </TooltipProvider>
    </QueryClientProvider>
  )
}
