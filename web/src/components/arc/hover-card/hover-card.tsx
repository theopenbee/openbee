"use client";

import { forwardRef, useCallback, useEffect, useId, useLayoutEffect, useRef, useState, useSyncExternalStore } from "react";
import type { FocusEvent, HTMLAttributes, PointerEvent, ReactElement, ReactNode } from "react";
import * as PopoverPrimitive from "@radix-ui/react-popover";
import { AnimatePresence, animate, motion, useMotionValue, usePresence, useReducedMotion } from "motion/react";
import { motionTokens } from "../lib/motion-tokens";
import styles from "./hover-card.module.css";

const OPEN_DELAY = 500;
const CLOSE_DELAY = 140;
const SKIP_WINDOW = 300;
const INTENT = 80;
const TRAVEL = 4;
const enter = [...motionTokens.ease.enter] as [number, number, number, number];
const standard = [...motionTokens.ease.standard] as [number, number, number, number];

type Snapshot = { id: string | null; instant: boolean };
const IDLE: Snapshot = { id: null, instant: false };
let snapshot = IDLE;
let closedAt = -Infinity;
const listeners = new Set<() => void>();
const cards = {
  subscribe(listener: () => void) { listeners.add(listener); return () => { listeners.delete(listener); }; },
  get: () => snapshot,
  server: () => IDLE,
  warm: () => snapshot.id !== null || performance.now() - closedAt < SKIP_WINDOW,
  open(id: string) { if (snapshot.id === id) return; snapshot = { id, instant: cards.warm() }; listeners.forEach(listener => listener()); },
  close(id: string) { if (snapshot.id !== id) return; snapshot = IDLE; closedAt = performance.now(); listeners.forEach(listener => listener()); },
};

export interface HoverCardProps {
  children: ReactElement;
  content: ReactNode;
  side?: "top" | "bottom" | "left" | "right";
  align?: "start" | "center" | "end";
  openDelay?: number;
  closeDelay?: number;
  className?: string;
}

interface SurfaceProps extends Omit<HTMLAttributes<HTMLDivElement>, "onDrag" | "onDragStart" | "onDragEnd" | "onAnimationStart"> { instant: boolean; reduced: boolean }

const Surface = forwardRef<HTMLDivElement, SurfaceProps>(function Surface({ instant, reduced, className, style, children, ...props }, forwardedRef) {
  const [isPresent, safeToRemove] = usePresence();
  const node = useRef<HTMLDivElement | null>(null);
  const progress = useMotionValue(0);
  const x = useMotionValue(0), y = useMotionValue(0), scale = useMotionValue(1), opacity = useMotionValue(0);
  const [still] = useState(() => instant || reduced);
  const setRef = useCallback((element: HTMLDivElement | null) => {
    node.current = element;
    if (typeof forwardedRef === "function") forwardedRef(element); else if (forwardedRef) forwardedRef.current = element;
  }, [forwardedRef]);

  useEffect(() => {
    let cancelled = false;
    const apply = (value: number) => {
      const side = node.current?.dataset.side;
      const travel = still ? 0 : (1 - value) * TRAVEL;
      x.set(side === "left" ? travel : side === "right" ? -travel : 0);
      y.set(side === "top" ? travel : side === "bottom" ? -travel : 0);
      scale.set(still ? 1 : .96 + .04 * value);
    };
    const unsubscribe = progress.on("change", apply);
    apply(progress.get());
    const controls = isPresent
      ? animate(progress, 1, still ? { duration: .14, ease: enter } : motionTokens.spring.smooth)
      : animate(progress, 0, { duration: .12, ease: standard });
    const fade = isPresent
      ? animate(opacity, 1, { duration: reduced ? .15 : still ? .09 : .16, ease: enter })
      : animate(opacity, 0, { duration: reduced ? .1 : .12, ease: standard });
    if (!isPresent) fade.then(() => { if (!cancelled) safeToRemove?.(); });
    return () => { cancelled = true; unsubscribe(); controls.stop(); fade.stop(); };
  }, [isPresent, still, reduced, progress, x, y, scale, opacity, safeToRemove]);

  return <motion.div {...props} ref={setRef} className={[styles.card, className].filter(Boolean).join(" ")} style={{ ...style, x, y, scale, opacity }}>
    {children}
  </motion.div>;
});

export function HoverCard({ children, content, side = "bottom", align = "start", openDelay = OPEN_DELAY, closeDelay = CLOSE_DELAY, className }: HoverCardProps) {
  const id = useId();
  const cardId = `${id}-card`;
  const state = useSyncExternalStore(cards.subscribe, cards.get, cards.server);
  const open = state.id === id;
  const reduced = useReducedMotion() ?? false;
  const triggerRef = useRef<HTMLDivElement>(null);
  const timers = useRef({ open: 0, close: 0 });
  const inside = useRef({ trigger: false, card: false, focus: false });
  const dismissed = useRef(false);
  const pointerType = useRef("");

  useEffect(() => {
    const pending = timers.current;
    return () => { window.clearTimeout(pending.open); window.clearTimeout(pending.close); cards.close(id); };
  }, [id]);

  const clearTimers = () => { window.clearTimeout(timers.current.open); window.clearTimeout(timers.current.close); };
  const show = () => { clearTimers(); dismissed.current = false; cards.open(id); };
  const hide = () => { clearTimers(); cards.close(id); };
  const scheduleOpen = () => {
    window.clearTimeout(timers.current.close);
    if (dismissed.current || cards.get().id === id) return;
    window.clearTimeout(timers.current.open);
    timers.current.open = window.setTimeout(() => cards.open(id), cards.warm() ? INTENT : openDelay);
  };
  const scheduleClose = () => {
    window.clearTimeout(timers.current.open);
    const { trigger, card, focus } = inside.current;
    if (trigger || card || focus) return;
    window.clearTimeout(timers.current.close);
    timers.current.close = window.setTimeout(() => cards.close(id), closeDelay);
  };
  const mouse = (event: PointerEvent) => event.pointerType !== "touch";

  return <PopoverPrimitive.Root open={open} onOpenChange={next => { if (!next) hide(); }}>
    <PopoverPrimitive.Anchor asChild ref={triggerRef} aria-describedby={open ? cardId : undefined} data-hover-card={open ? "open" : "closed"}
      onPointerEnter={(event: PointerEvent) => { if (!mouse(event)) return; inside.current.trigger = true; scheduleOpen(); }}
      onPointerLeave={(event: PointerEvent) => { if (!mouse(event)) return; inside.current.trigger = false; dismissed.current = false; scheduleClose(); }}
      onPointerDown={(event: PointerEvent) => { pointerType.current = event.pointerType; }}
      onClick={() => {
        const isOpen = cards.get().id === id;
        if (pointerType.current === "touch") { if (isOpen) hide(); else show(); } else if (!isOpen) show();
        pointerType.current = "";
      }}
      onFocus={(event: FocusEvent<HTMLElement>) => { if (!event.currentTarget.matches(":focus-visible")) return; inside.current.focus = true; show(); }}
      onBlur={() => { inside.current.focus = false; dismissed.current = false; scheduleClose(); }}>
      {children}
    </PopoverPrimitive.Anchor>
    <AnimatePresence>
      {open && <PopoverPrimitive.Portal forceMount key="card">
        <PopoverPrimitive.Content forceMount asChild id={cardId} role="tooltip" side={side} align={align} sideOffset={8} collisionPadding={12}
          onOpenAutoFocus={event => event.preventDefault()} onCloseAutoFocus={event => event.preventDefault()}
          onEscapeKeyDown={() => { dismissed.current = true; }}
          onInteractOutside={event => { if (triggerRef.current?.contains(event.target as Node)) event.preventDefault(); }}
          onPointerEnter={(event: PointerEvent) => { if (!mouse(event)) return; inside.current.card = true; window.clearTimeout(timers.current.close); }}
          onPointerLeave={(event: PointerEvent) => { if (!mouse(event)) return; inside.current.card = false; scheduleClose(); }}>
          <Surface instant={state.instant} reduced={reduced} className={className}>{content}</Surface>
        </PopoverPrimitive.Content>
      </PopoverPrimitive.Portal>}
    </AnimatePresence>
  </PopoverPrimitive.Root>;
}

export interface HoverCardStat { label: string; value: string | number }

export interface HoverCardProfileProps {
  name: string;
  role?: string;
  avatar?: string | ReactNode;
  bio?: string;
  stats?: HoverCardStat[];
  meta?: ReactNode;
}

function Portrait({ src }: { src: string }) {
  const image = useRef<HTMLImageElement>(null);
  useLayoutEffect(() => { const node = image.current; if (node && !node.complete) node.dataset.loading = ""; }, [src]);
  return <img ref={image} src={src} alt="" width={48} height={48} decoding="async" onLoad={event => { delete event.currentTarget.dataset.loading; }} />;
}

export function HoverCardProfile({ name, role, avatar, bio, stats = [], meta }: HoverCardProfileProps) {
  const reduced = useReducedMotion() ?? false;
  const initials = name.trim().split(/\s+/).slice(0, 2).map(part => part[0]?.toUpperCase()).join("");
  const settle = (order: number) => reduced ? {} : { initial: { opacity: 0, y: 4 }, animate: { opacity: 1, y: 0 }, transition: { duration: motionTokens.duration.standard, ease: enter, delay: .04 + order * motionTokens.stagger.item } };
  return <div className={styles.profile}>
    <motion.div className={styles.head} {...settle(0)}>
      <span className={styles.avatar} aria-hidden="true">{typeof avatar === "string" ? <Portrait src={avatar} /> : avatar ?? <span className={styles.initials}>{initials}</span>}</span>
      <span className={styles.identity}><span className={styles.name}>{name}</span>{role && <span className={styles.role}>{role}</span>}</span>
    </motion.div>
    {bio && <motion.p className={styles.bio} {...settle(1)}>{bio}</motion.p>}
    {stats.length > 0 && <motion.dl className={styles.stats} {...settle(2)}>
      {stats.map(stat => <div key={stat.label} className={styles.stat}><dt>{stat.label}</dt><dd>{stat.value}</dd></div>)}
    </motion.dl>}
    {meta && <motion.div className={styles.meta} {...settle(3)}>{meta}</motion.div>}
  </div>;
}

export default HoverCard;
