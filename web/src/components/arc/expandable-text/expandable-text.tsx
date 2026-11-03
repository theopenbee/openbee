"use client";

import { useEffect, useId, useLayoutEffect, useRef, useState, useSyncExternalStore } from "react";
import type { CSSProperties, FocusEvent, HTMLAttributes, ReactNode } from "react";
import { ChevronDown } from "lucide-react";
import { animate, motion, useMotionValue, useReducedMotion, useTransform } from "motion/react";
import type { AnimationPlaybackControls } from "motion/react";
import { useTranslation } from "react-i18next";
import styles from "./expandable-text.module.css";

export interface ExpandableTextProps extends Omit<HTMLAttributes<HTMLDivElement>, "children"> {
  children: ReactNode;
  lines?: number;
  expanded?: boolean;
  defaultExpanded?: boolean;
  onExpandedChange?: (expanded: boolean) => void;
  moreLabel?: string;
  lessLabel?: string;
}

const noop = () => () => {};
const ease = [0.32, 0.5, 0.2, 1] as const;
const openDuration = (distance: number) => 0.5 + Math.min(distance / 1600, 0.1);
const closeDuration = (distance: number) => 0.4 + Math.min(distance / 2000, 0.05);

function useReduced() {
  const hydrated = useSyncExternalStore(noop, () => true, () => false);
  const reduced = useReducedMotion() ?? false;
  return hydrated && reduced;
}

export function ExpandableText({ children, lines = 3, expanded: expandedProp, defaultExpanded = false, onExpandedChange, moreLabel, lessLabel, className, style, ...props }: ExpandableTextProps) {
  const { t } = useTranslation();
  const [uncontrolled, setUncontrolled] = useState(defaultExpanded);
  const expanded = expandedProp ?? uncontrolled;
  const [overflowing, setOverflowing] = useState(true);
  const [animating, setAnimating] = useState(false);
  const [settled, setSettled] = useState(expanded);
  const reduced = useReduced();
  const contentId = useId();
  const root = useRef<HTMLDivElement>(null);
  const content = useRef<HTMLDivElement>(null);
  const inner = useRef<HTMLDivElement>(null);
  const probe = useRef<HTMLSpanElement>(null);
  const clampHeight = useRef(0);
  const range = useRef({ from: 0, to: 0 });
  const running = useRef<AnimationPlaybackControls | null>(null);
  const lastExpanded = useRef(expanded);
  const progress = useMotionValue(expanded ? 1 : 0);
  const height = useMotionValue<number | "auto">("auto");
  const rotate = useTransform(progress, [0, 1], [0, 180]);
  const maskImage = useTransform(progress, value => {
    const fade = Math.pow(1 - Math.min(Math.max(value, 0), 1), 1.35);
    return fade <= 0.002 ? "none" : `linear-gradient(to bottom, #000 calc(100% - 1.6lh), rgb(0 0 0 / ${(1 - fade * 0.96).toFixed(3)}) 100%)`;
  });

  useEffect(() => progress.on("change", value => {
    if (!running.current) return;
    const { from, to } = range.current;
    height.set(from + (to - from) * value);
  }), [progress, height]);

  useLayoutEffect(() => {
    const probeNode = probe.current, innerNode = inner.current;
    if (!probeNode || !innerNode) return;
    const measure = () => {
      clampHeight.current = probeNode.getBoundingClientRect().height;
      setOverflowing(innerNode.getBoundingClientRect().height > clampHeight.current + 1);
    };
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measure);
    observer.observe(probeNode);
    observer.observe(innerNode);
    return () => observer.disconnect();
  }, [lines]);

  useLayoutEffect(() => {
    if (lastExpanded.current === expanded) return;
    lastExpanded.current = expanded;
    const node = content.current, innerNode = inner.current;
    running.current?.stop();
    running.current = null;
    if (!node || !innerNode || reduced) { progress.set(expanded ? 1 : 0); height.set("auto"); setAnimating(false); setSettled(expanded); return; }
    const full = innerNode.getBoundingClientRect().height;
    const clamp = Math.min(clampHeight.current, full);
    const current = node.getBoundingClientRect().height;
    range.current = { from: clamp, to: full };
    const span = Math.max(full - clamp, 1);
    progress.jump(Math.min(Math.max((current - clamp) / span, 0), 1));
    node.style.height = `${current}px`;
    height.set(current);
    setAnimating(true);
    const target = expanded ? 1 : 0;
    const distance = Math.abs(target - progress.get()) * span;
    running.current = animate(progress, target, {
      duration: expanded ? openDuration(distance) : closeDuration(distance),
      ease: [...ease],
      onComplete: () => { running.current = null; height.set("auto"); setAnimating(false); setSettled(expanded); },
    });
  }, [expanded, reduced, progress, height]);

  useEffect(() => () => running.current?.stop(), []);

  const setExpanded = (next: boolean) => {
    if (next === expanded) return;
    const node = root.current;
    if (!next && node && node.getBoundingClientRect().top < 0) node.scrollIntoView({ block: "start", behavior: reduced ? "auto" : "smooth" });
    if (expandedProp === undefined) setUncontrolled(next);
    onExpandedChange?.(next);
  };

  const onFocusCapture = (event: FocusEvent<HTMLDivElement>) => {
    props.onFocusCapture?.(event);
    if (expanded || !overflowing || event.target === content.current || !content.current?.contains(event.target)) return;
    const box = content.current?.getBoundingClientRect(), target = event.target.getBoundingClientRect();
    if (box && target.bottom > box.bottom + 1) setExpanded(true);
  };

  const clamped = !settled && !animating && overflowing;
  const masked = overflowing && (!settled || animating);
  return <div {...props} ref={root} onFocusCapture={onFocusCapture} className={[styles.root, className].filter(Boolean).join(" ")} style={{ ...style, "--expandable-text-lines": lines } as CSSProperties}>
    <motion.div
      ref={content}
      id={contentId}
      className={styles.content}
      data-clamped={clamped ? "" : undefined}
      style={{ height, WebkitMaskImage: masked ? maskImage : "none", maskImage: masked ? maskImage : "none" }}
    >
      <span ref={probe} className={styles.probe} aria-hidden="true" />
      <div ref={inner} className={styles.inner}>{children}</div>
    </motion.div>
    {overflowing && <button type="button" className={styles.toggle} aria-expanded={expanded} aria-controls={contentId} onClick={() => setExpanded(!expanded)}>
      <span className={styles.labels}>
        <span className={styles.label} data-active={!expanded || undefined} aria-hidden={expanded || undefined}>{moreLabel ?? t("arc.expandableText.more", "Read more")}</span>
        <span className={styles.label} data-active={expanded || undefined} aria-hidden={!expanded || undefined}>{lessLabel ?? t("arc.expandableText.less", "Show less")}</span>
      </span>
      <motion.span className={styles.icon} aria-hidden="true" style={{ rotate }}>
        <ChevronDown width={16} height={16} strokeWidth={1.75} />
      </motion.span>
    </button>}
  </div>;
}

export default ExpandableText;
