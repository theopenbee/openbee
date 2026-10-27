"use client";

import { useState } from "react";
import type { ReactNode } from "react";
import * as AccordionPrimitive from "@radix-ui/react-accordion";
import { ChevronDown } from "lucide-react";
import { motion, useReducedMotion } from "motion/react";
import type { TargetAndTransition, Variants } from "motion/react";
import { motionTokens } from "../lib/motion-tokens";
import styles from "./accordion.module.css";

export interface AccordionItem { title: string; content: ReactNode; }
export interface AccordionProps {
  items: AccordionItem[];
  defaultOpen?: number;
  size?: "md" | "lg";
}

const panelOpen: TargetAndTransition = { height: "auto", opacity: 1, visibility: "visible" };
const panelClosed: TargetAndTransition = { height: 0, opacity: 0, transitionEnd: { visibility: "hidden" } };
const contentOpen: TargetAndTransition = { y: 0, filter: "blur(0px)", transitionEnd: { filter: "none" } };
const contentClosed: TargetAndTransition = { y: -6, filter: `blur(${motionTokens.blur.subtle}px)` };
const panelMotion: Variants = {
  open: { ...panelOpen, transition: { height: motionTokens.spring.smooth, opacity: { duration: motionTokens.duration.standard, ease: [...motionTokens.ease.enter] } } },
  closed: { ...panelClosed, transition: { height: motionTokens.spring.smooth, opacity: { duration: motionTokens.duration.fast, ease: [...motionTokens.ease.standard] } } },
};
const contentMotion: Variants = {
  open: { ...contentOpen, transition: { y: motionTokens.spring.smooth, filter: { duration: motionTokens.duration.standard, ease: [...motionTokens.ease.enter] } } },
  closed: { ...contentClosed, transition: { duration: motionTokens.duration.fast, ease: [...motionTokens.ease.standard] } },
};
const panelStill: Variants = { open: { ...panelOpen, transition: { duration: 0 } }, closed: { ...panelClosed, transition: { duration: 0 } } };
const contentStill: Variants = { open: { ...contentOpen, transition: { duration: 0 } }, closed: { ...contentClosed, transition: { duration: 0 } } };

export function Accordion({ items, defaultOpen = 0, size = "md" }: AccordionProps) {
  const initialValue = defaultOpen >= 0 && defaultOpen < items.length ? String(defaultOpen) : "";
  const [openValue, setOpenValue] = useState(initialValue);
  const reduced = useReducedMotion();
  return <AccordionPrimitive.Root className={[styles.accordion, size === "lg" ? styles.lg : ""].filter(Boolean).join(" ")} type="single" collapsible value={openValue} onValueChange={setOpenValue}>
    {items.map((item, index) => { const open = openValue === String(index); return <AccordionPrimitive.Item className={styles.item} value={String(index)} key={`${item.title}-${index}`}>
      <AccordionPrimitive.Header className={styles.header}>
        <AccordionPrimitive.Trigger className={styles.trigger}>
          <span>{item.title}</span><motion.span className={styles.icon} initial={false} animate={{ rotate: open ? 180 : 0 }} transition={reduced ? { duration: 0 } : motionTokens.spring.snappy}><ChevronDown width={17} height={17} aria-hidden="true" /></motion.span>
        </AccordionPrimitive.Trigger>
      </AccordionPrimitive.Header>
      <AccordionPrimitive.Content forceMount asChild>
        <motion.div className={styles.panel} initial={false} animate={open ? "open" : "closed"} variants={reduced ? panelStill : panelMotion}>
          <motion.div className={styles.panelInner} variants={reduced ? contentStill : contentMotion}>{item.content}</motion.div>
        </motion.div>
      </AccordionPrimitive.Content>
    </AccordionPrimitive.Item>; })}
  </AccordionPrimitive.Root>;
}

export default Accordion;
