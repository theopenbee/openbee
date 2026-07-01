import { useNavigate } from "react-router-dom"
import { useQueryClient } from "@tanstack/react-query"
import { clearTokens } from "@/lib/auth"

// useLogout returns a handler that fully tears down the current session. It
// clears the auth tokens AND the React Query cache (current user, permissions,
// user/role lists) so a subsequent login in the same tab never inherits the
// previous user's state, then routes to the login page. Every logout entry
// point must go through this so the two concerns can't drift apart again.
export function useLogout() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  return () => {
    clearTokens()
    queryClient.clear()
    navigate("/login", { replace: true })
  }
}
