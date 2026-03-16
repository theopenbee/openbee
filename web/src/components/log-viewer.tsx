import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Streamdown } from "streamdown";
import { config } from "@/lib/config";

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
    <div className="border-l-2 border-blue-400 pl-3 my-2 text-sm text-gray-700 prose prose-sm max-w-none">
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
    <div className="my-1 border border-gray-200 rounded text-xs">
      <button
        className="w-full text-left px-3 py-1.5 flex items-center gap-2 hover:bg-gray-50 transition-colors"
        onClick={() => setOpen((v) => !v)}
      >
        <span className="font-mono text-yellow-600">{meta.icon}</span>
        <span className="font-semibold text-yellow-700">{entry.name}</span>
        <span className="text-gray-500 font-mono truncate flex-1">
          {summary}
        </span>
        <span className="text-gray-400 shrink-0">{open ? "▲" : "▼"}</span>
      </button>
      {open && (
        <div className="border-t border-gray-200 px-3 py-2 space-y-2">
          <div>
            <div className="text-gray-400 mb-1">{t("logViewer.input")}</div>
            <pre className="text-gray-700 overflow-x-auto">
              {JSON.stringify(entry.input, null, 2)}
            </pre>
          </div>
          {entry.result !== undefined && (
            <div>
              <div className="text-gray-400 mb-1">{t("logViewer.output")}</div>
              <pre
                className={`overflow-x-auto ${entry.isError ? "text-red-600" : "text-green-600"}`}
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

function ResultCard({
  entry,
}: {
  entry: Extract<ParsedEntry, { kind: "result" }>;
}) {
  return (
    <div className="my-2 bg-green-50 border border-green-300 rounded px-3 py-2 text-sm text-green-700">
      <span className="font-semibold text-green-600 mr-2">✓ Result</span>
      {entry.text}
    </div>
  );
}

// ─── Main component ───────────────────────────────────────────────────────────

interface LogViewerProps {
  executionId: string;
  status: string;
  logs: string | null | undefined;
  onComplete?: () => void;
}

export function LogViewer({ executionId, status, logs, onComplete }: LogViewerProps) {
  const [entries, setEntries] = useState<ParsedEntry[]>([]);
  const toolMapRef = useRef<Map<string, number>>(new Map());
  const logsEndRef = useRef<HTMLDivElement>(null);
  const { t } = useTranslation();

  // Real-time WebSocket stream
  useEffect(() => {
    const wsBase = config.apiUrl;
    const wsUrl =
      wsBase.replace(/^http/, "ws") + `/executions/${executionId}/logs`;
    const ws = new WebSocket(wsUrl);

    ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      if (data.type === "done" || data.type === "error") {
        onComplete?.();
      }
      setEntries((prev) => {
        const next = [...prev];
        appendEntry(data.content, data.type, next, toolMapRef.current);
        return next;
      });
    };

    ws.onerror = () => {};

    return () => ws.close();
  }, [executionId]);

  // Historical logs (completed executions)
  useEffect(() => {
    if (status !== "running" && logs) {
      const newEntries: ParsedEntry[] = [];
      const newToolMap = new Map<string, number>();
      logs
        .split("\n")
        .filter(Boolean)
        .forEach((line) => appendEntry(line, "stdout", newEntries, newToolMap));
      toolMapRef.current = newToolMap;
      setEntries(newEntries);
    }
  }, [status, logs]);

  useEffect(() => {
    logsEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [entries]);

  return (
    <div className="bg-white text-gray-800 font-mono text-sm rounded-lg">
      {entries.length === 0 && (
        <p className="text-gray-400">
          {status === "running" ? t("logViewer.waiting") : t("logViewer.noLogs")}
        </p>
      )}
      {entries.map((entry, i) => {
        if (entry.kind === "text")
          return <AssistantText key={i} text={entry.text} />;
        if (entry.kind === "tool")
          return <ToolCard key={entry.id} entry={entry} />;
        // if (entry.kind === "result") return <ResultCard key={i} entry={entry} />
        if (entry.kind === "result") return null;
        return (
          <div
            key={i}
            className={
              entry.logType === "stderr"
                ? "text-red-500"
                : entry.logType === "error"
                  ? "text-red-700 font-bold"
                  : "text-green-700"
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
