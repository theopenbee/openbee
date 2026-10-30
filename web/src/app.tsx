import { lazy, Suspense, useEffect } from "react"
import { HashRouter, Navigate, Routes, Route, useLocation, useParams } from "react-router-dom"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { Layout } from "@/components/layout"
import { AuthGuard } from "@/components/auth-guard"
import { Toaster } from "@/components/toaster"
import { useSettingsDialog, type SettingsSection } from "@/hooks/use-settings-dialog"
import { legacyRedirect } from "@/lib/routes"

const Login = lazy(() => import("@/pages/login").then(m => ({ default: m.Login })))
const Setup = lazy(() => import("@/pages/setup").then(m => ({ default: m.Setup })))
const ChatIndex = lazy(() => import("@/pages/chat").then(m => ({ default: m.ChatIndex })))
const Chat = lazy(() => import("@/pages/chat").then(m => ({ default: m.Chat })))
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

function LegacyRedirect() {
  const { pathname, search } = useLocation()
  return <Navigate to={legacyRedirect(pathname, search) ?? "/chat"} replace />
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
              <Route path="/chat" element={<ChatIndex />} />
              <Route path="/chat/:conversation" element={<Chat />} />
              <Route path="/workers" element={<LegacyRedirect />} />
              <Route path="/workers/create" element={<LegacyRedirect />} />
              <Route path="/workers/:id" element={<LegacyRedirect />} />
              <Route path="/departments" element={<LegacyRedirect />} />
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
