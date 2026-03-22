import { Outlet } from "react-router-dom"
import { Nav } from "./nav"

export function Layout() {
  return (
    <div className="antialiased min-h-screen">
      <Nav />
      <main className="max-w-7xl mx-auto px-6 py-8">
        <Outlet />
      </main>
    </div>
  )
}
