"use client";

import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { CircleAlert, Copy } from "lucide-react";
import { motionTokens } from "../lib/motion-tokens";
import { useCopyFeedback } from "../lib/use-copy-feedback";
import { useTranslation } from "react-i18next";
import styles from "./copy-button.module.css";

export interface CopyButtonProps {
  value: string;
  label?: string;
  className?: string;
  iconOnly?: boolean;
  variant?: "outline" | "plain";
  disabled?: boolean;
  onCopied?: () => void;
}

const ease = [...motionTokens.ease.standard] as [number, number, number, number];
const show = { duration: .16, ease } as const;
const hide = { duration: .1, ease } as const;
const instant = { duration: 0 } as const;

function DrawnCheck({ reduced }: { reduced: boolean }) {
  return <svg width={16} height={16} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round">
    <motion.path d="M4 12l5 5L20 6" initial={reduced ? false : { pathLength: 0 }} animate={{ pathLength: 1 }} transition={{ duration: .22, ease, delay: .04 }} />
  </svg>;
}

export function CopyButton({ value, label: labelProp, className, iconOnly = false, variant = "outline", disabled, onCopied }: CopyButtonProps) {
  const { state, copy } = useCopyFeedback();
  const reduced = useReducedMotion() ?? false;
  const { t } = useTranslation();
  const label = labelProp ?? t("arc.copyButton.copy", "Copy");
  const copiedText = t("arc.copyButton.copied", "Copied"), failedText = t("arc.copyButton.failed", "Failed");
  const text = state === "copied" ? copiedText : state === "error" ? failedText : label;

  async function handleCopy() {
    if (await copy(value)) onCopied?.();
  }

  return <><button
    type="button"
    className={[styles.button, iconOnly && styles.iconOnly, variant === "plain" && styles.plain, className].filter(Boolean).join(" ")}
    onClick={() => void handleCopy()}
    aria-label={label}
    data-copy-state={state}
    disabled={disabled}
  >
    <span className={styles.icon} aria-hidden="true">
      <AnimatePresence initial={false}>
        <motion.span key={state} className={styles.iconInner} data-state={state}
          initial={reduced ? { opacity: 0 } : { opacity: 0, scale: .85 }} animate={{ opacity: 1, scale: 1 }}
          exit={{ opacity: 0, transition: reduced ? instant : hide }} transition={reduced ? instant : show}>
          {state === "copied" ? <DrawnCheck reduced={reduced} /> : state === "error" ? <CircleAlert size={16} strokeWidth={1.75} /> : <Copy size={16} strokeWidth={1.75} />}
        </motion.span>
      </AnimatePresence>
    </span>
    {!iconOnly && <span className={styles.label} aria-hidden="true">
      <span className={styles.measure}>{label}</span><span className={styles.measure}>{copiedText}</span><span className={styles.measure}>{failedText}</span>
      <AnimatePresence initial={false}>
        <motion.span key={text} className={styles.text} initial={reduced ? { opacity: 0 } : { opacity: 0, y: 2 }} animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0, transition: reduced ? instant : hide }} transition={reduced ? instant : show}>{text}</motion.span>
      </AnimatePresence>
    </span>}
  </button><span className={styles.srOnly} role="status" aria-live="polite">{state === "idle" ? "" : state === "error" ? t("arc.copyButton.failedAnnouncement", "{{label}}: Could not copy", { label }) : t("arc.copyButton.copiedAnnouncement", "{{label}}: Copied", { label })}</span></>;
}
