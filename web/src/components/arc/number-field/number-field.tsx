"use client";

import { AnimatePresence, animate, motion, useMotionValue, useReducedMotion, useTransform, type MotionValue, type Variants } from "motion/react";
import { Minus, Plus } from "lucide-react";
import { Fragment, useEffect, useId, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent, type PointerEvent, type Ref } from "react";
import { motionTokens } from "../lib/motion-tokens";
import { useTranslation } from "react-i18next";
import styles from "./number-field.module.css";

export type NumberFieldAffix = string | ((value: number) => string);

export interface NumberFieldProps {
  label: string;
  value?: number;
  defaultValue?: number;
  onValueChange?: (value: number) => void;
  min?: number;
  max?: number;
  step?: number;
  largeStep?: number;
  description?: string;
  disabled?: boolean;
  id?: string;
  prefix?: NumberFieldAffix;
  suffix?: NumberFieldAffix;
  scrub?: boolean;
  locale?: string;
  formatOptions?: { minimumFractionDigits?: number; maximumFractionDigits?: number; useGrouping?: boolean };
  size?: NumberFieldSize;
  limitHint?: boolean | ((edge: "min" | "max", limit: number) => string);
}

export type NumberFieldSize = "sm" | "md" | "lg";

type Source = "button" | "key" | "scrub" | "type";
type Part = { key: string; digit: number } | { key: string; text: string };

const { spring, duration, blur } = motionTokens;
const enterEase = [...motionTokens.ease.enter] as [number, number, number, number];
const exitEase = [...motionTokens.ease.standard] as [number, number, number, number];
const DIGITS = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9];
const SCRUB_PX = 6, HOLD_DELAY = 400, HOLD_FASTEST = 40;
const LIMIT_PUSH = 240, LIMIT_HINT_MS = 1500, UNDER_WARN_MS = 700;
const PUSH_WINDOW = 700, PUSH_GAIN = .22, PUSH_CAP = 4;
const BUMP_VELOCITY = 130;
const kick = (() => { const root = (2 * Math.PI) / (spring.morph.visualDuration * 1.2); return { type: "spring" as const, stiffness: root * root, damping: 2 * (1 - spring.morph.bounce) * root }; })();

const decimalsOf = (value: number) => (String(value).split(".")[1] ?? "").length;
const escape = (text: string) => text.replace(/[.*+?^${}()|[\]\\-]/g, "\\$&");
const affixText = (affix: NumberFieldAffix | undefined, value: number) => typeof affix === "function" ? affix(value) : affix ?? "";
const rubber = (distance: number, limit = 10) => Math.sign(distance) * (1 - 1 / (Math.abs(distance) * .55 / limit + 1)) * limit;

function partsOf(value: number, format: Intl.NumberFormat): Part[] {
  const parts = format.formatToParts(value);
  let place = parts.reduce((count, part) => count + (part.type === "integer" ? part.value.length : 0), 0);
  let fraction = 0;
  return parts.flatMap((part, index): Part[] => {
    if (part.type === "integer") return [...part.value].map(char => ({ key: `i${--place}`, digit: Number(char) }));
    if (part.type === "fraction") return [...part.value].map(char => ({ key: `f${fraction++}`, digit: Number(char) }));
    return [{ key: part.type === "group" ? `g${place}` : part.type === "decimal" ? "d" : part.type === "minusSign" ? "m" : `${part.type}${index}`, text: part.value }];
  });
}

const slot: Variants = {
  enter: (direction: number) => ({ width: 0, scale: .6, opacity: 0, y: `${direction * .3}em`, filter: `blur(${blur.soft}px)` }),
  center: { width: "auto", scale: 1, opacity: 1, y: 0, filter: "blur(0px)", transitionEnd: { filter: "none" }, transition: { width: spring.morph, scale: spring.morph, opacity: spring.morph, y: spring.snappy, filter: { duration: .3, ease: exitEase } } },
  exit: (direction: number) => ({ width: 0, scale: .6, opacity: 0, y: `${direction * -.3}em`, filter: `blur(${blur.subtle}px)`, transition: { width: spring.smooth, scale: spring.smooth, y: { duration: duration.fast, ease: exitEase }, opacity: { duration: duration.instant }, filter: { duration: duration.instant } } }),
};
const rest = (opacity: number) => ({ width: "auto", scale: 1, opacity, y: 0, filter: "none" });
const still: Variants = { enter: rest(0), center: { ...rest(1), transition: { duration: duration.instant } }, exit: { width: 0, opacity: 0, transition: { duration: 0 } } };
const cut: Variants = { enter: rest(1), center: { ...rest(1), transition: { duration: 0 } }, exit: { width: 0, opacity: 0, transition: { duration: 0 } } };

function Glyph({ position, digit }: { position: MotionValue<number>; digit: number }) {
  const offset = (current: number) => ((((digit - current) % 10) + 15) % 10) - 5;
  const y = useTransform(position, current => `${offset(current) * 1.05}em`);
  const opacity = useTransform(position, current => Math.max(0, 1 - Math.abs(offset(current)) ** 1.5 * 1.1));
  const visibility = useTransform(position, current => Math.abs(offset(current)) >= 1 ? "hidden" : "visible");
  const filter = useTransform(position, current => { const distance = Math.abs(offset(current)); return distance < .02 || distance >= 1 ? "none" : `blur(${(distance * blur.soft * .75).toFixed(2)}px)`; });
  return <motion.span className={styles.glyph} style={{ y, opacity, filter, visibility }}>{digit}</motion.span>;
}

function Wheel({ digit, direction, instant }: { digit: number; direction: number; instant: boolean }) {
  const reduced = useReducedMotion();
  const position = useMotionValue(digit);
  const wheel = useRef({ digit, target: digit });
  useLayoutEffect(() => {
    const state = wheel.current;
    if (state.digit === digit) return;
    let delta = direction > 0 ? (digit - state.digit + 10) % 10 : -((state.digit - digit + 10) % 10);
    if (Math.abs(delta) > 5) delta -= Math.sign(delta) * 10;
    state.target += delta;
    state.digit = digit;
    if (instant || reduced) position.jump(state.target);
    else animate(position, state.target, spring.snappy);
  }, [digit, direction, instant, position, reduced]);
  return <><span className={styles.sizer}>0</span>{DIGITS.map(item => <Glyph key={item} position={position} digit={item} />)}</>;
}

function Digits({ value, format, direction, instant }: { value: number; format: Intl.NumberFormat; direction: number; instant: boolean }) {
  const reduced = useReducedMotion();
  const variants = instant ? cut : reduced ? still : slot;
  return <AnimatePresence initial={false} custom={direction}>{partsOf(value, format).map(part => <motion.span key={part.key} className={"digit" in part ? styles.column : styles.symbol} custom={direction} variants={variants} initial="enter" animate="center" exit="exit">
    {"digit" in part ? <Wheel digit={part.digit} direction={direction} instant={instant} /> : part.text}
  </motion.span>)}</AnimatePresence>;
}

function AffixText({ text, direction, className }: { text: string; direction: number; className?: string }) {
  const reduced = useReducedMotion();
  return <span className={className}><AnimatePresence initial={false} custom={direction}>{[...text].map((char, index) => <motion.span key={`${index}:${char}`} className={styles.char} custom={direction} variants={reduced ? still : slot} initial="enter" animate="center" exit="exit">{char}</motion.span>)}</AnimatePresence></span>;
}

const numberIn = (word?: string) => { const digits = word?.replace(/[^\d.-]/g, ""); return digits && /\d/.test(digits) && Number.isFinite(Number(digits)) ? Number(digits) : null; };
const directionsBetween = (from: string, to: string) => { const before = from.split(" "); return to.split(" ").map((word, index) => { const a = numberIn(before[index]), b = numberIn(word); return a !== null && b !== null && b < a ? -1 : 1; }); };

const wordRise: Variants = {
  enter: (direction: number) => ({ opacity: 0, y: `${direction * .3}em`, filter: `blur(${blur.soft}px)` }),
  center: { opacity: 1, y: 0, filter: "blur(0px)", transitionEnd: { filter: "none" }, transition: { duration: .22, ease: enterEase, opacity: spring.morph } },
  exit: (direction: number) => ({ opacity: 0, y: `${direction * -.3}em`, filter: `blur(${blur.subtle}px)`, transition: { duration: .14, ease: exitEase } }),
};
const wordFade: Variants = { enter: { opacity: 0, y: 0, filter: "none" }, center: { opacity: 1, y: 0, filter: "none", transition: { duration: duration.instant } }, exit: { opacity: 0, transition: { duration: 0 } } };

function WordSlot({ word, direction }: { word: string; direction: number }) {
  const reduced = useReducedMotion();
  const sizerRef = useRef<HTMLSpanElement>(null);
  const width = useMotionValue<number | "auto">("auto");
  const measured = useRef(false);
  useLayoutEffect(() => {
    const node = sizerRef.current;
    if (!node || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(() => {
      if (!measured.current || reduced) width.jump(node.offsetWidth);
      else animate(width, node.offsetWidth, spring.morph);
      measured.current = true;
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, [reduced, width]);
  return <motion.span className={styles.wordSlot} style={{ width }}>
    <span ref={sizerRef} className={styles.wordSizer}>{word}</span>
    <AnimatePresence initial={false} mode="popLayout" custom={direction}><motion.span key={word} className={styles.word} custom={direction} variants={reduced ? wordFade : wordRise} initial="enter" animate="center" exit="exit">{word}</motion.span></AnimatePresence>
  </motion.span>;
}

function MotionText({ text }: { text: string }) {
  const [trail, setTrail] = useState({ text, directions: [] as number[] });
  if (trail.text !== text) setTrail({ text, directions: directionsBetween(trail.text, text) });
  const directions = trail.text === text ? trail.directions : directionsBetween(trail.text, text);
  const words = text.split(" ");
  return <><span className={styles.srOnly}>{text}</span><span className={styles.words} aria-hidden="true">{words.map((word, index) => <Fragment key={index}>{index > 0 && " "}<WordSlot word={word} direction={directions[index] ?? 1} /></Fragment>)}</span></>;
}

function FieldMessage({ id, text, className }: { id?: string; text?: string; className?: string }) {
  return <AnimatePresence initial={false}>{text ? <MessageRow key="message" id={id} text={text} className={className} /> : null}</AnimatePresence>;
}

function MessageRow({ id, text, className }: { id?: string; text: string; className?: string }) {
  const reduced = useReducedMotion();
  const copyRef = useRef<HTMLSpanElement>(null);
  const [height, setHeight] = useState<number | "auto">("auto");
  useLayoutEffect(() => {
    const node = copyRef.current;
    if (!node || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(([entry]) => setHeight(entry?.borderBoxSize?.[0]?.blockSize ?? node.offsetHeight));
    observer.observe(node);
    return () => observer.disconnect();
  }, []);
  return <motion.span className={styles.messageSlot} initial={{ height: 0, opacity: 0 }} animate={{ height, opacity: 1 }} exit={{ height: 0, opacity: 0, transition: reduced ? { duration: 0 } : { height: spring.smooth, opacity: { duration: duration.instant } } }} transition={reduced ? { duration: 0 } : { height: spring.smooth, opacity: { duration: duration.fast } }}>
    <motion.span ref={copyRef} id={id} className={className} initial={reduced ? false : { y: "0.35em", filter: `blur(${blur.soft}px)` }} animate={{ y: 0, filter: "blur(0px)" }} transition={{ duration: reduced ? 0 : duration.standard, ease: enterEase }}><MotionText text={text} /></motion.span>
  </motion.span>;
}

function LimitNote({ id, edge, text }: { id: string; edge: 1 | -1 | 0; text: string }) {
  const reduced = useReducedMotion();
  return <AnimatePresence initial={false}>{edge !== 0 && <motion.span key="limit" id={id} className={styles.limit}
    initial={reduced ? { opacity: 0 } : { opacity: 0, y: `${edge * .45}em`, filter: `blur(${blur.subtle}px)` }}
    animate={{ opacity: 1, y: 0, filter: "blur(0px)", transitionEnd: { filter: "none" } }}
    exit={{ opacity: 0, transition: { duration: reduced ? duration.instant : duration.standard, ease: exitEase } }}
    transition={reduced ? { duration: duration.instant } : { y: spring.snappy, opacity: { duration: duration.fast }, filter: { duration: duration.fast } }}>
    {text}
  </motion.span>}</AnimatePresence>;
}

function StepButton({ toward, label, disabled, controls, limit, pressed, iconRef, onPress, onRelease, onActivate }: { toward: 1 | -1; label: string; disabled?: boolean; controls: string; limit: boolean; pressed: boolean; iconRef: Ref<HTMLSpanElement>; onPress: () => void; onRelease: () => void; onActivate: () => void }) {
  const Icon = toward > 0 ? Plus : Minus;
  const { t } = useTranslation();
  return <button type="button" className={styles.step} disabled={disabled} aria-controls={controls} aria-label={toward > 0 ? t("arc.numberField.increase", "Increase {{label}}", { label }) : t("arc.numberField.decrease", "Decrease {{label}}", { label })} aria-disabled={limit || undefined} data-pressed={pressed || undefined}
    onPointerDown={event => { if (event.button === 0) onPress(); }} onPointerUp={onRelease} onPointerLeave={onRelease} onPointerCancel={onRelease}
    onMouseDown={event => event.preventDefault()} onClick={event => { if (event.detail === 0) onActivate(); }} onContextMenu={event => event.preventDefault()}>
    <span ref={iconRef} className={styles.icon}><Icon size={16} strokeWidth={1.75} aria-hidden="true" /></span>
  </button>;
}

export function NumberField({ label, value, defaultValue = 0, onValueChange, min = 0, max = Number.MAX_SAFE_INTEGER, step: stepProp = 1, largeStep, description, disabled, id, prefix, suffix, scrub = false, locale = "en-US", formatOptions, size = "md", limitHint = true }: NumberFieldProps) {
  const reduced = useReducedMotion();
  const { t } = useTranslation();
  const generatedId = useId();
  const inputId = id ?? generatedId;
  const hintId = description ? `${inputId}-description` : undefined;
  const limitId = `${inputId}-limit`;
  const step = stepProp > 0 ? stepProp : 1;
  const [internal, setInternal] = useState(defaultValue);
  const current = value ?? internal;
  const [draft, setDraft] = useState("");
  const [editing, setEditing] = useState(false);
  const [pressed, setPressed] = useState(0);
  const [scrubbing, setScrubbing] = useState(false);
  const [announcement, setAnnouncement] = useState("");
  const [pushed, setPushed] = useState<1 | -1 | 0>(0);
  const [underWarn, setUnderWarn] = useState(false);
  const controlRef = useRef<HTMLDivElement>(null);
  const minusIcon = useRef<HTMLSpanElement>(null);
  const plusIcon = useRef<HTMLSpanElement>(null);
  const pushedTimer = useRef<number | undefined>(undefined);
  const underTimer = useRef<number | undefined>(undefined);
  const strainTimer = useRef<number | undefined>(undefined);
  const effort = useRef({ edge: 0, count: 0, at: 0 });
  const inputRef = useRef<HTMLInputElement>(null);
  const groupRef = useRef<HTMLSpanElement>(null);
  const latest = useRef(current);
  const editStart = useRef(current);
  const hold = useRef<number | undefined>(undefined);
  const drag = useRef<{ pointer: number; x: number; from: number; active: boolean } | null>(null);
  const suppressClick = useRef(false);
  const shiftFrom = useRef<number | null>(null);
  const selectNext = useRef(false);
  const live = useRef<(direction: 1 | -1, steps: number, source: Source) => boolean>(() => false);
  const bumpY = useMotionValue(0);
  const scrubX = useMotionValue(0);
  const shiftX = useMotionValue(0);
  const x = useTransform(() => scrubX.get() + shiftX.get());

  const base = Number.isFinite(min) ? min : 0;
  const decimals = Math.max(decimalsOf(step), decimalsOf(base));
  const minFraction = formatOptions?.minimumFractionDigits ?? decimals;
  const maxFraction = Math.max(minFraction, formatOptions?.maximumFractionDigits ?? decimals);
  const grouping = formatOptions?.useGrouping ?? true;
  const format = useMemo(() => new Intl.NumberFormat(locale, { minimumFractionDigits: minFraction, maximumFractionDigits: maxFraction, useGrouping: grouping, numberingSystem: "latn" }), [locale, minFraction, maxFraction, grouping]);
  const symbols = useMemo(() => { const parts = new Intl.NumberFormat(locale).formatToParts(-1234.5); return { group: parts.find(part => part.type === "group")?.value ?? ",", decimal: parts.find(part => part.type === "decimal")?.value ?? "." }; }, [locale]);
  const round = (next: number) => Number(next.toFixed(decimals)) || 0;
  const clamp = (next: number) => Math.min(max, Math.max(min, next));
  const snap = (next: number) => round(base + Math.round((next - base) / step) * step);
  const stepFrom = (from: number, steps: number) => { const index = (from - base) / step; return round(base + ((steps > 0 ? Math.floor(index + 1e-7) : Math.ceil(index - 1e-7)) + steps) * step); };
  const spoken = (next: number) => `${affixText(prefix, next)}${format.format(next)}${affixText(suffix, next)}`.trim();
  function parse(text: string) {
    const normalized = text.split(symbols.group).join("").replace(symbols.decimal, ".").replace(/[^\d.-]/g, "");
    if (!/\d/.test(normalized)) return null;
    const parsed = Number(normalized);
    return Number.isFinite(parsed) ? parsed : null;
  }
  const typed = editing ? parse(draft) : null;
  const shown = typed ?? current;
  const [trail, setTrail] = useState({ value: shown, direction: 1 as 1 | -1 });
  if (trail.value !== shown) setTrail({ value: shown, direction: shown > trail.value ? 1 : -1 });
  const direction = trail.value === shown ? trail.direction : shown > trail.value ? 1 : -1;

  useEffect(() => () => { window.clearTimeout(hold.current); window.clearTimeout(pushedTimer.current); window.clearTimeout(underTimer.current); window.clearTimeout(strainTimer.current); }, []);

  function strain(edge: 1 | -1, source: Source) {
    const now = performance.now(), push = effort.current;
    push.count = push.edge === edge && now - push.at < PUSH_WINDOW ? push.count + 1 : 1;
    push.edge = edge;
    push.at = now;
    const control = controlRef.current;
    if (control) {
      control.dataset.strain = edge > 0 ? "max" : "min";
      window.clearTimeout(strainTimer.current);
      strainTimer.current = window.setTimeout(() => { delete control.dataset.strain; }, 420);
    }
    if (!reduced) {
      animate(bumpY, 0, { ...kick, velocity: -edge * BUMP_VELOCITY * (1 + Math.min(push.count - 1, PUSH_CAP) * PUSH_GAIN) });
      const icon = (edge > 0 ? plusIcon : minusIcon).current;
      if (icon && (source === "button" || source === "key")) animate(icon, { x: [0, -2.5, 2.5, -1.5, 1, 0] }, { duration: .32, ease: "easeOut" });
    }
    if (limitHint) {
      setPushed(edge);
      window.clearTimeout(pushedTimer.current);
      pushedTimer.current = window.setTimeout(() => setPushed(0), LIMIT_HINT_MS);
    }
  }
  function commitValue(next: number, source: Source) {
    if (!Number.isFinite(next)) return false;
    const clamped = clamp(next);
    const limit = Math.sign(next - clamped);
    if (limit && source !== "scrub") { strain(limit > 0 ? 1 : -1, source); setAnnouncement(limit > 0 ? t("arc.numberField.atMaximum", "{{value}}, maximum", { value: spoken(clamped) }) : t("arc.numberField.atMinimum", "{{value}}, minimum", { value: spoken(clamped) })); }
    else if (source === "button" && clamped !== latest.current) setAnnouncement(spoken(clamped));
    if (clamped === latest.current) return false;
    latest.current = clamped;
    if (value === undefined) setInternal(clamped);
    onValueChange?.(clamped);
    return true;
  }
  const nudge = (toward: 1 | -1, steps: number, source: Source) => commitValue(stepFrom(latest.current, toward * steps), source);
  useLayoutEffect(() => { latest.current = current; live.current = nudge; });

  function captureShift() {
    const group = groupRef.current;
    if (group && shiftFrom.current === null) shiftFrom.current = group.getBoundingClientRect().left - scrubX.get() - shiftX.get();
  }
  useLayoutEffect(() => {
    const from = shiftFrom.current, group = groupRef.current;
    shiftFrom.current = null;
    if (selectNext.current) { selectNext.current = false; const input = inputRef.current; if (input && document.activeElement === input) { const end = input.value.length; input.setSelectionRange(end, end); } }
    if (from === null || !group) return;
    const delta = from - (group.getBoundingClientRect().left - scrubX.get() - shiftX.get());
    if (Math.abs(delta) < .5) return;
    shiftX.set(shiftX.get() + delta);
    if (reduced) shiftX.jump(0); else animate(shiftX, 0, spring.morph);
  });

  function clearUnder() {
    window.clearTimeout(underTimer.current);
    setUnderWarn(false);
  }
  function commitDraft() {
    if (!editing) return;
    captureShift();
    setEditing(false);
    clearUnder();
    const parsed = parse(draft);
    if (parsed !== null) commitValue(snap(parsed), "type");
  }
  function stopHold() {
    window.clearTimeout(hold.current);
    hold.current = undefined;
    setPressed(0);
  }
  function startHold(toward: 1 | -1, amount: number, source: Source) {
    stopHold();
    commitDraft();
    const steps = Math.max(1, Math.round(amount / step));
    if (source === "button") setPressed(toward);
    let count = 0;
    const tick = () => {
      const moved = live.current(toward, steps, source);
      hold.current = window.setTimeout(tick, moved ? Math.max(HOLD_FASTEST, 150 * .86 ** ++count) : LIMIT_PUSH);
    };
    hold.current = window.setTimeout(tick, nudge(toward, steps, source) ? HOLD_DELAY : LIMIT_PUSH + 120);
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    const large = largeStep ?? step * 10;
    const move = ({ ArrowUp: [1, event.shiftKey ? large : step], ArrowDown: [-1, event.shiftKey ? large : step], PageUp: [1, large], PageDown: [-1, large] } as Record<string, [1 | -1, number]>)[event.key];
    if (move) { event.preventDefault(); if (!event.repeat) startHold(move[0], move[1], "key"); return; }
    if (!editing && ((event.key === "Home" && Number.isFinite(min)) || (event.key === "End" && max < Number.MAX_SAFE_INTEGER))) { event.preventDefault(); commitValue(event.key === "Home" ? min : max, "key"); return; }
    if (event.key === "Enter") { event.preventDefault(); if (editing) { selectNext.current = true; commitDraft(); } else event.currentTarget.select(); }
    if (event.key === "Escape" && editing) { event.preventDefault(); captureShift(); setEditing(false); clearUnder(); selectNext.current = true; commitValue(editStart.current, "type"); }
  }
  function onChange(text: string) {
    const allowed = new RegExp(`[^0-9${escape(symbols.group)}${decimals || maxFraction ? escape(symbols.decimal) : ""}${min < 0 ? "\\-" : ""}]`, "g");
    const next = text.replace(allowed, "");
    if (!editing) editStart.current = current;
    captureShift();
    setDraft(next);
    setEditing(true);
    const parsed = parse(next);
    if (parsed !== null && parsed >= min && parsed <= max && snap(parsed) === parsed) commitValue(parsed, "type");
    clearUnder();
    if (parsed !== null && parsed < min) underTimer.current = window.setTimeout(() => setUnderWarn(true), UNDER_WARN_MS);
  }

  function onScrubStart(event: PointerEvent<HTMLLabelElement>) {
    suppressClick.current = false;
    if (!scrub || disabled || event.button !== 0) return;
    commitDraft();
    drag.current = { pointer: event.pointerId, x: event.clientX, from: latest.current, active: false };
    event.currentTarget.setPointerCapture(event.pointerId);
  }
  function onScrubMove(event: PointerEvent<HTMLLabelElement>) {
    const state = drag.current;
    if (!state || state.pointer !== event.pointerId) return;
    const dx = event.clientX - state.x;
    if (!state.active) { if (Math.abs(dx) < 3) return; state.active = true; setScrubbing(true); }
    const travel = dx / SCRUB_PX, steps = Math.trunc(travel);
    commitValue(steps ? stepFrom(state.from, steps) : state.from, "scrub");
    const raw = state.from + travel * step;
    const over = raw > max ? (raw - max) / step * SCRUB_PX : raw < min ? (raw - min) / step * SCRUB_PX : 0;
    scrubX.set(reduced ? 0 : rubber(over));
  }
  function onScrubEnd(event: PointerEvent<HTMLLabelElement>) {
    const state = drag.current;
    if (!state || state.pointer !== event.pointerId) return;
    drag.current = null;
    if (!state.active) return;
    suppressClick.current = true;
    setScrubbing(false);
    animate(scrubX, 0, reduced ? { duration: 0 } : spring.snappy);
    setAnnouncement(spoken(latest.current));
  }

  const stepper = (toward: 1 | -1) => ({ toward, label, disabled, controls: inputId, limit: toward > 0 ? shown >= max : shown <= min, pressed: pressed === toward });
  const outside: 1 | -1 | 0 = typed === null ? 0 : typed > max ? 1 : typed < min && underWarn ? -1 : 0;
  const noteEdge = limitHint ? outside || pushed : 0;
  const noteLimit = noteEdge > 0 ? max : min;
  const noteText = !noteEdge ? "" : typeof limitHint === "function" ? limitHint(noteEdge > 0 ? "max" : "min", noteLimit) : noteEdge > 0 ? t("arc.numberField.max", "Max {{value}}", { value: spoken(noteLimit) }) : t("arc.numberField.min", "Min {{value}}", { value: spoken(noteLimit) });
  const describedBy = [hintId, outside && limitHint ? limitId : undefined].filter(Boolean).join(" ") || undefined;

  return <div className={styles.field} data-size={size}>
    <div className={styles.head}>
      <label htmlFor={inputId} className={styles.label} data-scrub={(scrub && !disabled) || undefined} onPointerDown={onScrubStart} onPointerMove={onScrubMove} onPointerUp={onScrubEnd} onPointerCancel={onScrubEnd} onClick={event => { if (suppressClick.current) { event.preventDefault(); suppressClick.current = false; } }}>{label}</label>
      <LimitNote id={limitId} edge={noteEdge} text={noteText} />
    </div>
    <div ref={controlRef} className={styles.control} data-scrubbing={scrubbing || undefined} data-disabled={disabled || undefined} data-warn={outside ? (outside > 0 ? "max" : "min") : undefined}>
      <StepButton {...stepper(-1)} iconRef={minusIcon} onPress={() => startHold(-1, step, "button")} onRelease={stopHold} onActivate={() => { commitDraft(); nudge(-1, 1, "button"); }} />
      <div className={styles.valueWrap} onMouseDown={event => { if (event.target !== inputRef.current) event.preventDefault(); }} onClick={event => { if (!disabled && event.target !== inputRef.current) { inputRef.current?.focus(); inputRef.current?.select(); } }}>
        <motion.span ref={groupRef} className={styles.value} style={{ x, y: bumpY }}>
          <AffixText text={affixText(prefix, shown)} direction={direction} className={styles.affix} />
          <span className={styles.number}>
            <span className={styles.display} data-hidden={editing || undefined} aria-hidden="true"><Digits value={shown} format={format} direction={direction} instant={editing} /></span>
            {editing && <span className={styles.mirror} aria-hidden="true">{draft}</span>}
            <input ref={inputRef} id={inputId} className={styles.input} type="text" role="spinbutton" inputMode={min < 0 ? "text" : decimals || maxFraction ? "decimal" : "numeric"} autoComplete="off" spellCheck={false}
              value={editing ? draft : format.format(current)} disabled={disabled} aria-describedby={describedBy} aria-invalid={outside ? true : undefined} aria-valuenow={current} aria-valuetext={spoken(current)}
              aria-valuemin={Number.isFinite(min) ? min : undefined} aria-valuemax={max < Number.MAX_SAFE_INTEGER ? max : undefined}
              onChange={event => onChange(event.currentTarget.value)} onKeyDown={onKeyDown} onKeyUp={event => { if (/^(Arrow(Up|Down)|Page(Up|Down))$/.test(event.key)) stopHold(); }} onBlur={() => { stopHold(); commitDraft(); }} />
          </span>
          <AffixText text={affixText(suffix, shown)} direction={direction} className={styles.affix} />
        </motion.span>
      </div>
      <StepButton {...stepper(1)} iconRef={plusIcon} onPress={() => startHold(1, step, "button")} onRelease={stopHold} onActivate={() => { commitDraft(); nudge(1, 1, "button"); }} />
    </div>
    <FieldMessage id={hintId} text={description} className={styles.hint} />
    <span className={styles.srOnly} aria-live="polite" aria-atomic="true">{announcement}</span>
  </div>;
}
