import { lazy, Suspense, useEffect } from "react"
import { HashRouter, Navigate, Routes, Route, useParams } from "react-router-dom"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { Layout } from "@/components/layout"
import { AuthGuard } from "@/components/auth-guard"
import { Toaster } from "@/components/toaster"
import { useSettingsDialog, type SettingsSection } from "@/hooks/use-settings-dialog"

const Login = lazy(() => import("@/pages/login").then(m => ({ default: m.Login })))
const Setup = lazy(() => import("@/pages/setup").then(m => ({ default: m.Setup })))
const Workers = lazy(() => import("@/pages/workers").then(m => ({ default: m.Workers })))
const CreateWorker = lazy(() => import("@/pages/create-worker").then(m => ({ default: m.CreateWorker })))
const WorkerDetail = lazy(() => import("@/pages/worker-detail").then(m => ({ default: m.WorkerDetail })))
const LocalChat = lazy(() => import("@/pages/local-chat").then(m => ({ default: m.LocalChat })))
const Departments = lazy(() => import("@/pages/departments").then(m => ({ default: m.Departments })))
const NotFound = lazy(() => import("@/pages/not-found").then(m => ({ default: m.NotFound })))

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
    },
  },
})

function SettingsRedirect({ section }: { section: SettingsSection }) {
  const { id } = useParams()
  const show = useSettingsDialog()
  useEffect(() => show({ section, plugin: id }), [show, section, id])
  return <Navigate to="/" replace />
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <Toaster />
      <HashRouter>
        <Suspense fallback={null}>
          <Routes>
            <Route path="/login" element={<Login />} />
            <Route path="/setup" element={<Setup />} />
            <Route element={<AuthGuard><Layout /></AuthGuard>}>
              <Route path="/" element={<Navigate to="/chat" replace />} />
              <Route path="/workers" element={<Workers />} />
              <Route path="/workers/create" element={<CreateWorker />} />
              <Route path="/workers/:id" element={<WorkerDetail />} />
              <Route path="/departments" element={<Departments />} />
              <Route path="/chat" element={<LocalChat />} />
              <Route path="/env" element={<SettingsRedirect section="env" />} />
              <Route path="/settings" element={<SettingsRedirect section="system" />} />
              <Route path="/plugins" element={<SettingsRedirect section="plugins" />} />
              <Route path="/plugins/:id" element={<SettingsRedirect section="plugins" />} />
            </Route>
            <Route path="*" element={<NotFound />} />
          </Routes>
        </Suspense>
      </HashRouter>
    </QueryClientProvider>
  )
}
