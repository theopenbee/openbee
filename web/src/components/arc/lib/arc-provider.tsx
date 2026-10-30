"use client";

import { createContext, createElement, useContext, useMemo, type AnchorHTMLAttributes, type ComponentType, type ImgHTMLAttributes, type ReactNode, type Ref } from "react";

export interface ArcLinkProps extends AnchorHTMLAttributes<HTMLAnchorElement> {
  href: string;
  ref?: Ref<HTMLAnchorElement>;
}

export interface ArcImageProps extends Omit<ImgHTMLAttributes<HTMLImageElement>, "src" | "alt" | "width" | "height" | "loading" | "srcSet"> {
  src: string;
  alt: string;
  width?: number;
  height?: number;
  fill?: boolean;
  priority?: boolean;
  loading?: "eager" | "lazy";
  ref?: Ref<HTMLImageElement>;
}

export type ArcLinkComponent = ComponentType<ArcLinkProps>;
export type ArcImageComponent = ComponentType<ArcImageProps>;

export function ArcAnchor(props: ArcLinkProps) {
  return <a {...props} />;
}

const fillStyle = { position: "absolute", inset: 0, width: "100%", height: "100%" } as const;

export function ArcImg({ alt, fill, priority, loading, style, ...props }: ArcImageProps) {
  return <img alt={alt} decoding="async" {...props} loading={loading ?? (priority ? "eager" : "lazy")} fetchPriority={priority ? "high" : props.fetchPriority} style={fill ? { ...fillStyle, ...style } : style} />;
}

interface ArcComponents {
  link: ArcLinkComponent;
  image: ArcImageComponent;
}

const ArcContext = createContext<ArcComponents>({ link: ArcAnchor, image: ArcImg });

export interface ArcProviderProps {
  link?: ArcLinkComponent;
  image?: ArcImageComponent;
  children: ReactNode;
}

export function ArcProvider({ link, image, children }: ArcProviderProps) {
  const parent = useContext(ArcContext);
  const value = useMemo(() => ({ link: link ?? parent.link, image: image ?? parent.image }), [link, image, parent]);
  return <ArcContext.Provider value={value}>{children}</ArcContext.Provider>;
}

export const useArcLink = () => useContext(ArcContext).link;
export const useArcImage = () => useContext(ArcContext).image;

export function ArcLink(props: ArcLinkProps) {
  return createElement(useArcLink(), props);
}

export function ArcImage(props: ArcImageProps) {
  return createElement(useArcImage(), props);
}
