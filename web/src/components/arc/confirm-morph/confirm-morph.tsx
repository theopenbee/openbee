"use client";

import { useCallback, useEffect, useId, useImperativeHandle, useLayoutEffect, useRef, useState, useSyncExternalStore } from "react";
import type { KeyboardEvent as ReactKeyboardEvent, PointerEvent as ReactPointerEvent, ReactNode, Ref } from "react";
import { AnimatePresence, animate, motion, useIsPresent, useMotionValue, useMotionValueEvent, useReducedMotion, useTransform } from "motion/react";
import type { AnimationPlaybackControls, MotionValue, Transition, Variants } from "motion/react";
import { CircleAlert, LoaderCircle } from "lucide-react";
import { motionTokens } from "../lib/motion-tokens";
import { useTranslation } from "react-i18next";
import styles from "./confirm-morph.module.css";

export type ConfirmMorphState = "idle" | "confirming" | "pending" | "done" | "error";

export type ConfirmMorphMode = "press" | "hold" | "countdown";

export interface ConfirmMorphProps {
  label: ReactNode;
  icon?: ReactNode;
  prompt?: ReactNode;
  confirmLabel?: string;
  cancelLabel?: string;
  pendingLabel?: string;
  doneLabel?: string;
  errorLabel?: string;
  retryLabel?: string;
  undoLabel?: string;
  undoingLabel?: string;
  tone?: "danger" | "neutral";
  confirmMode?: ConfirmMorphMode;
  holdDuration?: number;
  countdown?: number;
  countdownLabel?: string;
  onConfirm?: () => void | Promise<unknown>;
  onUndo?: () => void | Promise<unknown>;
  onCancel?: () => void;
  state?: ConfirmMorphState;
  defaultState?: ConfirmMorphState;
  onStateChange?: (state: ConfirmMorphState) => void;
  confirmTimeout?: number;
  resultTimeout?: number;
  cancelOnOutsidePress?: boolean;
  disabled?: boolean;
  className?: string;
  ref?: Ref<HTMLDivElement>;
}

const ARM_MS = 280;
const { blur } = motionTokens;
type Bezier = [number, number, number, number];
const enter = [...motionTokens.ease.enter] as Bezier;
const standard = [...motionTokens.ease.standard] as Bezier;
const physical = ({ visualDuration, bounce }: { visualDuration: number; bounce: number }): Transition => {
  const root = 2 * Math.PI / (visualDuration * 1.2);
  return { type: "spring", stiffness: root * root, damping: 2 * (1 - bounce) * root, mass: 1 };
};
const GROW = physical(motionTokens.spring.morph), SHRINK = physical(motionTokens.spring.smooth);

const subscribe = () => () => {};
function useReducedFlag() {
  const hydrated = useSyncExternalStore(subscribe, () => true, () => false);
  return !!useReducedMotion() && hydrated;
}

const faceVariants: Variants = {
  hidden: { opacity: 1 },
  shown: { opacity: 1, transition: { staggerChildren: motionTokens.stagger.item, delayChildren: .05 } },
  gone: { opacity: 0, filter: `blur(${blur.subtle}px)`, transition: { duration: motionTokens.duration.instant, ease: standard } },
};
const partVariants: Variants = {
  hidden: { opacity: 0, filter: `blur(${blur.soft}px)` },
  shown: { opacity: 1, filter: "blur(0px)", transition: { duration: .26, ease: enter } },
};
const reducedFace: Variants = { hidden: { opacity: 0 }, shown: { opacity: 1, transition: { duration: .14 } }, gone: { opacity: 0, transition: { duration: .1 } } };
const reducedPart: Variants = { hidden: { opacity: 1 }, shown: { opacity: 1 } };

type FaceKey = ConfirmMorphState | "counting";

function Face({ id, reduced, onSize, children, labelledBy }: { id: FaceKey; reduced: boolean; onSize: (id: FaceKey, width: number) => void; children: ReactNode; labelledBy?: string }) {
  const ref = useRef<HTMLDivElement>(null);
  const present = useIsPresent();
  useLayoutEffect(() => {
    const node = ref.current;
    if (!node || !present) return;
    const report = () => onSize(id, node.getBoundingClientRect().width);
    report();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(report);
    observer.observe(node);
    return () => observer.disconnect();
  }, [id, onSize, present]);
  return <motion.div ref={ref} className={styles.face} data-face={id} role={labelledBy ? "group" : undefined} aria-labelledby={labelledBy}
    variants={reduced ? reducedFace : faceVariants} initial="hidden" animate="shown" exit="gone" inert={!present || undefined}>{children}</motion.div>;
}

function Part({ reduced, className, children }: { reduced: boolean; className?: string; children: ReactNode }) {
  return <motion.span className={[styles.part, className].filter(Boolean).join(" ")} variants={reduced ? reducedPart : partVariants}>{children}</motion.span>;
}

function Check({ reduced }: { reduced: boolean }) {
  return <svg className={styles.check} viewBox="0 0 18 18" fill="none" aria-hidden="true">
    <motion.circle className={styles.checkDisc} cx="9" cy="9" r="8" style={{ transformOrigin: "9px 9px" }}
      initial={reduced ? false : { scale: .5, opacity: 0 }} animate={{ scale: 1, opacity: 1 }} transition={{ scale: physical(motionTokens.spring.snappy), opacity: { duration: .12 } }} />
    <motion.path className={styles.checkTick} d="M5.6 9.3 7.8 11.4 12.4 6.7" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round"
      initial={reduced ? false : { pathLength: 0 }} animate={{ pathLength: 1 }} transition={{ duration: .3, ease: enter, delay: .14 }} />
  </svg>;
}

function Ring({ progress }: { progress: MotionValue<number> }) {
  return <svg className={styles.ring} viewBox="0 0 16 16" fill="none" aria-hidden="true">
    <circle className={styles.ringTrack} cx="8" cy="8" r="6.25" strokeWidth="1.75" />
    <motion.circle className={styles.ringFill} cx="8" cy="8" r="6.25" strokeWidth="1.75" strokeLinecap="round" style={{ pathLength: progress, rotate: -90, transformOrigin: "8px 8px" }} />
  </svg>;
}

function Seconds({ value, reduced }: { value: number; reduced: boolean }) {
  return <span className={styles.seconds}>
    <AnimatePresence initial={false} mode="popLayout">
      <motion.span key={value} className={styles.digit}
        initial={reduced ? { opacity: 0 } : { opacity: 0, y: 6, filter: `blur(${blur.subtle}px)` }}
        animate={{ opacity: 1, y: 0, filter: "blur(0px)" }}
        exit={reduced ? { opacity: 0 } : { opacity: 0, y: -6, filter: `blur(${blur.subtle}px)` }}
        transition={{ duration: .22, ease: enter }}>{value}</motion.span>
    </AnimatePresence>
  </span>;
}

export function ConfirmMorph({
  label, icon, prompt, confirmLabel: confirmLabelProp, cancelLabel: cancelLabelProp, pendingLabel: pendingLabelProp, doneLabel: doneLabelProp, errorLabel: errorLabelProp,
  retryLabel: retryLabelProp, undoLabel: undoLabelProp, undoingLabel: undoingLabelProp, tone = "danger", confirmMode = "press", holdDuration = 1000, countdown = 3, countdownLabel,
  onConfirm, onUndo, onCancel, state: stateProp, defaultState = "idle", onStateChange, confirmTimeout = 6000, resultTimeout = 5000, cancelOnOutsidePress = true,
  disabled = false, className, ref,
}: ConfirmMorphProps) {
  const reduced = useReducedFlag();
  const { t } = useTranslation();
  const confirmLabel = confirmLabelProp ?? t("arc.confirmMorph.confirm", "Delete"), cancelLabel = cancelLabelProp ?? t("arc.confirmMorph.cancel", "Cancel"), pendingLabel = pendingLabelProp ?? t("arc.confirmMorph.pending", "Deleting"), doneLabel = doneLabelProp ?? t("arc.confirmMorph.done", "Deleted");
  const errorLabel = errorLabelProp ?? t("arc.confirmMorph.error", "Couldn’t finish"), retryLabel = retryLabelProp ?? t("arc.confirmMorph.retry", "Retry"), undoLabel = undoLabelProp ?? t("arc.confirmMorph.undo", "Undo"), undoingLabel = undoingLabelProp ?? t("arc.confirmMorph.undoing", "Restoring");
  const shownCountdown = countdownLabel ?? t("arc.confirmMorph.countdown", "{{pending}} in", { pending: pendingLabel, confirm: confirmLabel });
  const uid = useId();
  const promptId = `${uid}-prompt`;
  const holdHintId = `${uid}-hold`;
  const rootRef = useRef<HTMLDivElement>(null);
  useImperativeHandle(ref, () => rootRef.current as HTMLDivElement, []);

  const [inner, setInner] = useState<ConfirmMorphState>(defaultState);
  const state = stateProp ?? inner;
  const [working, setWorking] = useState<"confirm" | "undo">("confirm");
  const [counting, setCounting] = useState(false);
  const [announcement, setAnnouncement] = useState("");
  const faceKey: FaceKey = state === "confirming" && counting ? "counting" : state;

  const live = useRef({ state, onStateChange, controlled: stateProp !== undefined });
  useLayoutEffect(() => { live.current = { state, onStateChange, controlled: stateProp !== undefined }; });
  const pendingFocus = useRef(false);
  const run = useRef(0);
  const askedAt = useRef(0);

  const keepFocus = useCallback(() => {
    const root = rootRef.current;
    pendingFocus.current = !!root && (root.contains(document.activeElement) || document.activeElement === document.body);
  }, []);

  const go = useCallback((next: ConfirmMorphState) => {
    if (next !== "confirming") setCounting(false);
    if (next === live.current.state) return;
    keepFocus();
    if (next === "confirming") askedAt.current = performance.now();
    if (!live.current.controlled) setInner(next);
    live.current.state = next;
    live.current.onStateChange?.(next);
  }, [keepFocus]);

  const [seenState, setSeenState] = useState(state);
  if (seenState !== state) {
    setSeenState(state);
    if (state !== "confirming") setCounting(false);
  }

  const toIdle = useCallback(() => { run.current++; go("idle"); }, [go]);

  const perform = useCallback(async (kind: "confirm" | "undo") => {
    const handler = kind === "confirm" ? onConfirm : onUndo;
    const token = ++run.current;
    setWorking(kind);
    setCounting(false);
    let result: void | Promise<unknown> | undefined;
    try { result = handler?.(); }
    catch { go("error"); setAnnouncement(errorLabel); return; }
    if (result && typeof (result as Promise<unknown>).then === "function") {
      go("pending");
      setAnnouncement(kind === "confirm" ? pendingLabel : undoingLabel);
      try { await result; }
      catch {
        if (token !== run.current) return;
        go("error");
        setAnnouncement(errorLabel);
        return;
      }
      if (token !== run.current) return;
    }
    if (kind === "undo") { go("idle"); setAnnouncement(t("arc.confirmMorph.undone", "Undone")); return; }
    go("done");
    setAnnouncement(onUndo ? t("arc.confirmMorph.undoAvailable", "{{done}}. {{undo}} is available.", { done: doneLabel, undo: undoLabel }) : doneLabel);
  }, [doneLabel, errorLabel, go, onConfirm, onUndo, pendingLabel, t, undoLabel, undoingLabel]);

  const cancel = useCallback(() => { onCancel?.(); toIdle(); setAnnouncement(t("arc.confirmMorph.cancelled", "Cancelled")); }, [onCancel, t, toIdle]);
  const expire = useRef(() => {});
  useLayoutEffect(() => { expire.current = () => { if (live.current.state === "confirming") cancel(); else toIdle(); }; });

  const width = useMotionValue<number | "auto">("auto");
  const target = useRef(0);
  const flight = useRef(0);
  const onFaceSize = useCallback((_id: FaceKey, w: number) => {
    if (Math.abs(w - target.current) < .25) return;
    const from = target.current;
    target.current = w;
    if (!from || reduced) { flight.current++; width.jump("auto"); return; }
    const now = width.get();
    if (now === "auto") width.jump(from);
    const token = ++flight.current;
    animate(width, w, w > (typeof now === "number" ? now : from) ? GROW : SHRINK).then(() => { if (token === flight.current) width.jump("auto"); });
  }, [reduced, width]);

  const drain = useMotionValue(1);
  const clock = useRef<AnimationPlaybackControls | null>(null);
  const holds = useRef({ hover: false, hidden: false });
  const timeout = state === "confirming" && !counting ? confirmTimeout : state === "done" || state === "error" ? resultTimeout : 0;
  const sync = useCallback(() => {
    const control = clock.current;
    if (!control) return;
    const held = holds.current.hover || holds.current.hidden;
    if (held) control.pause(); else control.play();
  }, []);
  useEffect(() => {
    if (!timeout) return;
    drain.jump(1);
    const control = animate(drain, 0, { duration: timeout / 1000, ease: "linear" });
    clock.current = control;
    control.then(() => { if (clock.current === control) { clock.current = null; expire.current(); } });
    sync();
    return () => { if (clock.current === control) clock.current = null; control.stop(); };
  }, [drain, state, sync, timeout]);
  useEffect(() => {
    const onVisibility = () => { holds.current.hidden = document.hidden; sync(); };
    document.addEventListener("visibilitychange", onVisibility);
    return () => document.removeEventListener("visibilitychange", onVisibility);
  }, [sync]);

  const remaining = useMotionValue(1);
  const [seconds, setSeconds] = useState(countdown);
  const performRef = useRef(perform);
  useLayoutEffect(() => { performRef.current = perform; });
  useMotionValueEvent(remaining, "change", value => setSeconds(Math.max(1, Math.ceil(value * countdown))));
  useEffect(() => {
    if (!counting) return;
    remaining.jump(1);
    const control = animate(remaining, 0, { duration: countdown, ease: "linear" });
    control.then(() => { void performRef.current("confirm"); });
    return () => control.stop();
  }, [counting, countdown, remaining]);

  const hold = useMotionValue(0);
  const holding = useRef<AnimationPlaybackControls | null>(null);
  const fillClip = useTransform(hold, value => `inset(0 ${(1 - value) * 100}% 0 0 round 999px)`);
  const startHold = () => {
    if (holding.current) return;
    const control = animate(hold, 1, { duration: holdDuration / 1000 * (1 - hold.get()), ease: "linear" });
    holding.current = control;
    control.then(() => {
      if (holding.current !== control) return;
      holding.current = null;
      void perform("confirm");
    });
  };
  const releaseHold = () => {
    const control = holding.current;
    if (!control) return;
    holding.current = null;
    control.stop();
    if (hold.get() < 1) {
      animate(hold, 0, reduced ? { duration: 0 } : { type: "spring", visualDuration: .3, bounce: 0 });
      setAnnouncement(t("arc.confirmMorph.keepHolding", "Keep holding to {{action}}", { action: confirmLabel.toLowerCase() }));
    }
  };
  useEffect(() => { if (state !== "confirming") { holding.current?.stop(); holding.current = null; hold.jump(0); } }, [hold, state]);

  const answer = () => {
    if (performance.now() - askedAt.current < ARM_MS) return;
    if (confirmMode === "countdown") {
      keepFocus();
      setCounting(true);
      setAnnouncement(t("arc.confirmMorph.countdownAnnouncement", "{{label}} {{seconds}} seconds", { label: shownCountdown, seconds: countdown }));
      return;
    }
    void perform("confirm");
  };

  useEffect(() => {
    if (state !== "confirming" || !cancelOnOutsidePress) return;
    const down = (event: PointerEvent) => { if (!rootRef.current?.contains(event.target as Node)) cancel(); };
    document.addEventListener("pointerdown", down);
    return () => document.removeEventListener("pointerdown", down);
  }, [cancel, cancelOnOutsidePress, state]);

  useLayoutEffect(() => {
    if (!pendingFocus.current) return;
    pendingFocus.current = false;
    const root = rootRef.current;
    if (!root) return;
    const face = root.querySelector<HTMLElement>(`[data-face="${faceKey}"]`);
    const autofocus = face?.querySelector<HTMLElement>("[data-autofocus]:not(:disabled)");
    (autofocus ?? root).focus({ preventScroll: true });
  }, [faceKey]);

  const onKeyDown = (event: ReactKeyboardEvent<HTMLDivElement>) => {
    if (event.key === "Escape") {
      if (state === "confirming") { event.preventDefault(); event.stopPropagation(); cancel(); }
      else if (state === "done" || state === "error") { event.preventDefault(); event.stopPropagation(); toIdle(); }
      return;
    }
    if (event.key === "Enter" && state === "confirming" && !event.repeat && !(event.target instanceof HTMLButtonElement)) {
      event.preventDefault();
      if (counting) void perform("confirm");
      else if (confirmMode !== "hold") answer();
    }
  };

  const holdKeys = (event: ReactKeyboardEvent<HTMLButtonElement>, down: boolean) => {
    if (event.key !== "Enter" && event.key !== " ") return;
    event.preventDefault();
    if (down) { if (!event.repeat) startHold(); }
    else releaseHold();
  };
  const holdPointer = (event: ReactPointerEvent<HTMLButtonElement>) => {
    if (event.button !== 0) return;
    event.currentTarget.setPointerCapture?.(event.pointerId);
    startHold();
  };

  const shownPrompt = prompt ?? <>{label}{t("arc.confirmMorph.question", "?")}</>;
  const confirmButton = confirmMode === "hold"
    ? <button type="button" className={styles.confirm} data-autofocus aria-describedby={holdHintId}
      onPointerDown={holdPointer} onPointerUp={releaseHold} onPointerCancel={releaseHold} onLostPointerCapture={releaseHold}
      onKeyDown={event => holdKeys(event, true)} onKeyUp={event => holdKeys(event, false)} onBlur={releaseHold} onContextMenu={event => event.preventDefault()}>
      <span>{confirmLabel}</span>
      <motion.span className={styles.fill} style={{ clipPath: fillClip }} aria-hidden="true"><span>{confirmLabel}</span></motion.span>
    </button>
    : <button type="button" className={styles.confirm} data-autofocus onClick={answer} onKeyDown={event => { if (event.repeat) event.preventDefault(); }}>{confirmLabel}</button>;

  const face = (() => {
    switch (faceKey) {
      case "confirming": return <>
        <Part reduced={reduced} className={styles.prompt}><span id={promptId}>{shownPrompt}</span></Part>
        <Part reduced={reduced}><button type="button" className={styles.secondary} onClick={cancel}>{cancelLabel}</button></Part>
        <Part reduced={reduced}>{confirmButton}</Part>
        {confirmMode === "hold" && <span id={holdHintId} className={styles.srOnly}>{t("arc.confirmMorph.holdHint", "Press and hold")}</span>}
      </>;
      case "counting": return <>
        <Part reduced={reduced} className={styles.status}>
          <Ring progress={remaining} />
          <span className={styles.statusText}>{shownCountdown} <Seconds value={seconds} reduced={reduced} /></span>
        </Part>
        <Part reduced={reduced}><button type="button" className={styles.secondary} onClick={cancel}>{cancelLabel}</button></Part>
      </>;
      case "pending": return <Part reduced={reduced} className={styles.status}>
        <LoaderCircle className={styles.spinner} size={16} strokeWidth={1.75} aria-hidden="true" />
        <span className={styles.statusText}>{working === "undo" ? undoingLabel : pendingLabel}</span>
      </Part>;
      case "done": return <>
        <Part reduced={reduced} className={styles.status}><Check reduced={reduced} /><span className={styles.statusText}>{doneLabel}</span></Part>
        {onUndo && <Part reduced={reduced}><button type="button" className={styles.secondary} data-autofocus onClick={() => void perform("undo")}>{undoLabel}</button></Part>}
      </>;
      case "error": return <>
        <Part reduced={reduced} className={styles.status}><CircleAlert className={styles.alert} size={16} strokeWidth={1.75} aria-hidden="true" /><span className={styles.statusText}>{errorLabel}</span></Part>
        <Part reduced={reduced}><button type="button" className={styles.secondary} data-autofocus onClick={() => void perform(working)}>{retryLabel}</button></Part>
      </>;
      default: return <Part reduced={reduced}><button type="button" className={styles.trigger} data-autofocus disabled={disabled}
        onClick={() => { setAnnouncement(typeof shownPrompt === "string" ? shownPrompt : ""); go("confirming"); }}>
        {icon && <span className={styles.icon} aria-hidden="true">{icon}</span>}
        <span>{label}</span>
      </button></Part>;
    }
  })();

  return <div ref={rootRef} className={[styles.root, className].filter(Boolean).join(" ")} data-state={state} data-face={faceKey} data-tone={tone} data-mode={confirmMode}
    data-disabled={disabled || undefined} tabIndex={-1} onKeyDown={onKeyDown} aria-busy={state === "pending" || undefined}
    onPointerEnter={() => { holds.current.hover = true; sync(); }} onPointerLeave={() => { holds.current.hover = false; sync(); }}
    onPointerCancel={() => { holds.current.hover = false; sync(); }}>
    <motion.div className={styles.surface} style={{ width }}>
      <AnimatePresence initial={false}>
        <Face key={faceKey} id={faceKey} reduced={reduced} onSize={onFaceSize} labelledBy={faceKey === "confirming" ? promptId : undefined}>{face}</Face>
      </AnimatePresence>
    </motion.div>
    <span className={styles.srOnly} role="status" aria-live="polite">{announcement}</span>
  </div>;
}

export default ConfirmMorph;
