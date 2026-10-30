import { useId, type ReactNode } from "react"
import { LogoFull } from "@/components/brand/logo"
import { LanguageMenu } from "@/components/language-menu"
import styles from "./auth-layout.module.css"

const RINGS = Array.from({ length: 12 }, (_, index) => ({ r: 232 + index * 64, major: index % 3 === 2 }))

export function AuthBackdrop() {
  return (
    <div className={styles.backdrop} aria-hidden="true">
      <svg className={styles.lines}>
        <svg x="50%" y="50%" overflow="visible">
          <line className={styles.axis} x1="-2400" x2="2400" y1="0" y2="0" />
          <line className={styles.axis} x1="0" x2="0" y1="-2400" y2="2400" />
          {RINGS.map((ring) => (
            <circle
              key={ring.r}
              className={ring.major ? `${styles.ring} ${styles.ringMajor}` : styles.ring}
              r={ring.r}
              cx="0"
              cy="0"
            />
          ))}
        </svg>
      </svg>
    </div>
  )
}

export function AuthLayout({
  title,
  description,
  children,
}: {
  title: string
  description?: string
  children: ReactNode
}) {
  const headingId = useId()

  return (
    <div className={styles.screen}>
      <AuthBackdrop />
      <div className={styles.toolbar}>
        <LanguageMenu />
      </div>
      <main className={styles.column}>
        <LogoFull className="h-8" />
        <section className={styles.card} aria-labelledby={headingId}>
          <header className={styles.heading}>
            <h1 id={headingId} className={styles.title}>{title}</h1>
            {description ? <p className={styles.description}>{description}</p> : null}
          </header>
          {children}
        </section>
      </main>
    </div>
  )
}
