import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Streamdown } from "streamdown";
import { api } from "@/lib/api";
import type { ExecutionStatus } from "@/lib/types";

// ─── Data model ───────────────────────────────────────────────────────────────

type ParsedEntry =
  | { kind: "text"; text: string }
  | {
      kind: "tool";
      id: string;
      name: string;
      input: unknown;
      result?: string;
      isError?: boolean;
    }
  | { kind: "result"; text: string; subtype: string }
  | { kind: "raw"; content: string; logType: string };

// ─── Parse helpers ────────────────────────────────────────────────────────────

interface ClaudeStreamEvent {
  type: string;
  subtype?: string;
  message?: {
    content: Array<{
      type: string;
      text?: string;
      id?: string;
      name?: string;
      input?: unknown;
      tool_use_id?: string;
      content?: string | unknown;
      is_error?: boolean;
    }>;
  };
  result?: string;
}

function parseStreamLine(line: string): ClaudeStreamEvent | null {
  try {
    const obj = JSON.parse(line);
    if (obj && typeof obj.type === "string") return obj as ClaudeStreamEvent;
    return null;
  } catch {
    return null;
  }
}

function getToolMeta(name: string): {
  icon: string;
  summary: (input: unknown) => string;
} {
  const truncate = (s: string, n = 80) =>
    s.length > n ? s.slice(0, n) + "…" : s;

  switch (name) {
    case "Bash":
      return {
        icon: "$",
        summary: (input) =>
          truncate((input as { command?: string })?.command ?? ""),
      };
    case "Read":
    case "Write":
    case "Edit":
    case "Glob":
    case "Grep":
      return {
        icon: "📄",
        summary: (input) => {
          const i = input as Record<string, string>;
          return truncate(
            i?.file_path ?? i?.pattern ?? i?.path ?? JSON.stringify(input),
          );
        },
      };
    case "WebSearch":
    case "WebFetch":
      return {
        icon: "🌐",
        summary: (input) => {
          const i = input as Record<string, string>;
          return truncate(i?.query ?? i?.url ?? JSON.stringify(input));
        },
      };
    default:
      return {
        icon: "🔧",
        summary: (input) => truncate(JSON.stringify(input)),
      };
  }
}

function extractToolResultText(content: unknown): string {
  if (typeof content === "string") return content;
  if (Array.isArray(content)) {
    const texts = content
      .filter((c): c is { type: string; text: string } =>
        typeof c === "object" &&
        c !== null &&
        "text" in c &&
        typeof (c as Record<string, unknown>).text === "string",
      )
      .map((c) => c.text);
    if (texts.length > 0) return texts.join("\n");
  }
  return JSON.stringify(content, null, 2);
}

// ─── Entry accumulator (pure, no React state) ─────────────────────────────────

function appendEntry(
  content: string,
  logType: string,
  entries: ParsedEntry[],
  toolMap: Map<string, number>,
) {
  if (logType === "stdout") {
    const event = parseStreamLine(content);
    if (event) {
      if (event.type === "assistant" && event.message?.content) {
        for (const block of event.message.content) {
          if (block.type === "text" && block.text) {
            entries.push({ kind: "text", text: block.text });
          } else if (block.type === "tool_use" && block.id && block.name) {
            toolMap.set(block.id, entries.length);
            entries.push({
              kind: "tool",
              id: block.id,
              name: block.name,
              input: block.input,
            });
          }
        }
        return;
      }

      if (event.type === "user" && event.message?.content) {
        for (const block of event.message.content) {
          if (block.type === "tool_result" && block.tool_use_id) {
            const idx = toolMap.get(block.tool_use_id);
            if (idx !== undefined) {
              const existing = entries[idx];
              if (existing?.kind === "tool") {
                entries[idx] = {
                  ...existing,
                  result: extractToolResultText(block.content),
                  isError: block.is_error,
                };
              }
            }
          }
        }
        return;
      }

      if (event.type === "result") {
        entries.push({
          kind: "result",
          text: event.result ?? "",
          subtype: event.subtype ?? "",
        });
        return;
      }

      if (event.type === "system") return;
      if (event.type === "rate_limit_event") return;
    }
  }

  entries.push({ kind: "raw", content, logType });
}

// ─── Sub-components ───────────────────────────────────────────────────────────

function AssistantText({ text }: { text: string }) {
  return (
    <div className="border-l-2 border-primary pl-3 my-2 text-sm prose prose-sm dark:prose-invert max-w-none">
      <Streamdown mode="static">{text}</Streamdown>
    </div>
  );
}

function ToolCard({
  entry,
}: {
  entry: Extract<ParsedEntry, { kind: "tool" }>;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const meta = getToolMeta(entry.name);
  const summary = meta.summary(entry.input);

  return (
    <div className="my-1 bg-secondary ring-1 ring-foreground/5 rounded-lg text-xs">
      <button
        aria-expanded={open}
        aria-label={open ? t("logViewer.collapse", { name: entry.name }) : t("logViewer.expand", { name: entry.name })}
        className="w-full text-left px-3 py-1.5 flex items-center gap-2 hover:bg-foreground/5 transition-colors rounded-lg"
        onClick={() => setOpen((v) => !v)}
      >
        <span className="font-mono text-primary" aria-hidden="true">{meta.icon}</span>
        <span className="font-semibold text-primary">{entry.name}</span>
        <span className="text-muted-foreground font-mono truncate flex-1">
          {summary}
        </span>
        <span className="text-muted-foreground shrink-0" aria-hidden="true">{open ? "▲" : "▼"}</span>
      </button>
      {open && (
        <div className="border-t border-border px-3 py-2 space-y-2">
          <div>
            <div className="text-muted-foreground mb-1">{t("logViewer.input")}</div>
            <pre className="text-foreground/80 overflow-x-auto bg-background rounded p-3 text-xs">
              {JSON.stringify(entry.input, null, 2)}
            </pre>
          </div>
          {entry.result !== undefined && (
            <div>
              <div className="text-muted-foreground mb-1">{t("logViewer.output")}</div>
              <pre
                className={`overflow-x-auto bg-background rounded p-3 text-xs ${entry.isError ? "text-destructive" : "text-status-idle"}`}
              >
                {entry.result}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// ─── Main component ───────────────────────────────────────────────────────────

interface LogViewerProps {
  executionId: string;
  status: ExecutionStatus;
  onComplete?: () => void;
  autoScroll?: boolean;
}

export function LogViewer({ executionId, status, onComplete, autoScroll = true }: LogViewerProps) {
  const [entries, setEntries] = useState<ParsedEntry[]>([]);
  const toolMapRef = useRef<Map<string, number>>(new Map());
  const logsEndRef = useRef<HTMLDivElement>(null);
  const parsedLengthRef = useRef<number>(0);
  const prevStatusRef = useRef<ExecutionStatus>(status);
  const { t } = useTranslation();

  useEffect(() => {
    setEntries([]);
    toolMapRef.current = new Map();
    parsedLengthRef.current = 0;
  }, [executionId]);

  useEffect(() => {
    const fetchLogs = async () => {
      try {
        const text = await api.executions.logs(executionId);
        if (text.length > parsedLengthRef.current) {
          const newContent = text.slice(parsedLengthRef.current);
          parsedLengthRef.current = text.length;
          setEntries((prev) => {
            const next = [...prev];
            newContent
              .split("\n")
              .filter(Boolean)
              .forEach((line) => appendEntry(line, "stdout", next, toolMapRef.current));
            return next;
          });
        }
      } catch {
        // ignore transient errors
      }
    };

    fetchLogs();

    if (status === "running" || status === "pending") {
      const interval = setInterval(fetchLogs, 2000);
      return () => clearInterval(interval);
    }
  }, [executionId, status]);

  useEffect(() => {
    if (prevStatusRef.current === "running" && status !== "running") {
      onComplete?.();
    }
    prevStatusRef.current = status;
  }, [status, onComplete]);

  useEffect(() => {
    if (autoScroll) {
      logsEndRef.current?.scrollIntoView({ behavior: "smooth" });
    }
  }, [entries, autoScroll]);

  return (
    <div className="bg-secondary/30 rounded-lg p-4 font-mono text-sm">
      {entries.length === 0 && (
        <p className="text-muted-foreground">
          {status === "running" ? (
            <span className="inline-flex items-center gap-2">
              <span className="w-1.5 h-1.5 rounded-full bg-primary animate-pulse-amber" />
              {t("logViewer.waiting")}
            </span>
          ) : (
            t("logViewer.noLogs")
          )}
        </p>
      )}
      {entries.map((entry, i) => {
        if (entry.kind === "text")
          return <AssistantText key={i} text={entry.text} />;
        if (entry.kind === "tool")
          return <ToolCard key={entry.id} entry={entry} />;
        if (entry.kind === "result")
          return (
            <div key={i} className="my-2 bg-primary/10 border border-primary/30 rounded-lg px-3 py-2 text-sm text-primary">
              <span className="font-semibold mr-2">✓ {t("logViewer.result")}</span>
              {entry.text}
            </div>
          );
        return (
          <div
            key={i}
            className={
              entry.logType === "stderr"
                ? "text-destructive"
                : entry.logType === "error"
                  ? "text-destructive font-bold"
                  : "text-muted-foreground"
            }
          >
            {entry.content}
          </div>
        );
      })}
      <div ref={logsEndRef} />
    </div>
  );
}
