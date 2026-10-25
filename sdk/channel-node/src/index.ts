import { randomBytes } from "node:crypto";
import { writeFile } from "node:fs/promises";
import { join } from "node:path";
import { format } from "node:util";

import {
  CODE_CONFIG_INVALID,
  CODE_INTERNAL_ERROR,
  CODE_METHOD_NOT_FOUND,
  CODE_REQUEST_CANCELLED,
  CODE_REQUEST_FAILED,
  IncomingMessage,
  MessageId,
  Peer,
  PROTOCOL_VERSION,
} from "./rpc.js";
import type {
  ChannelContext,
  ChannelDefinition,
  ChannelHandle,
  InitializeParams,
  LogFields,
  SendRequest,
} from "./types.js";

export * from "./types.js";
export { PROTOCOL_VERSION } from "./rpc.js";

export class ConfigError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ConfigError";
  }
}

export interface ChannelIO {
  input: NodeJS.ReadableStream;
  output: NodeJS.WritableStream;
}

function redirectConsole(): void {
  const toStderr = (...args: unknown[]) => {
    process.stderr.write(format(...args) + "\n");
  };
  console.log = toStderr;
  console.info = toStderr;
  console.debug = toStderr;
}

function installCrashHandlers(): void {
  const crash = (err: unknown) => {
    process.stderr.write(`fatal: ${err instanceof Error ? (err.stack ?? err.message) : String(err)}\n`);
    process.exit(1);
  };
  process.on("uncaughtException", crash);
  process.on("unhandledRejection", crash);
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

export function defineChannel(def: ChannelDefinition, io?: ChannelIO): void {
  const input = io?.input ?? process.stdin;
  const output = io?.output ?? process.stdout;
  if (output === process.stdout) {
    redirectConsole();
    installCrashHandlers();
  }

  const peer = new Peer(input, output);
  const inflight = new Map<MessageId, AbortController>();
  let ctx: ChannelContext | undefined;
  let handle: ChannelHandle | void = undefined;
  let stopping: Promise<void> | undefined;

  const stop = (): Promise<void> => {
    if (!stopping) {
      stopping = (async () => {
        for (const controller of inflight.values()) controller.abort();
        try {
          await handle?.stop?.();
        } catch (err) {
          process.stderr.write(`stop failed: ${errorMessage(err)}\n`);
        }
      })();
    }
    return stopping;
  };

  const exit = (code: number) => {
    output.write("", () => process.exit(code));
  };

  const log = (level: string) => (message: string, fields?: LogFields) => {
    void peer.notify("log", fields ? { level, message, fields } : { level, message });
  };

  const buildContext = (p: InitializeParams): ChannelContext => ({
    channelId: p.channel_id,
    config: p.config ?? {},
    botName: p.bot_name ?? "",
    language: p.language ?? "en",
    dataDir: p.data_dir,
    mediaDir: p.media_dir,
    openbeeVersion: p.openbee_version ?? "",
    dispatch: (message) => {
      void peer.notify("inbound", message);
    },
    saveMedia: async (data, ext) => {
      const suffix = ext ? (ext.startsWith(".") ? ext : "." + ext) : "";
      const path = join(p.media_dir, `${Date.now()}-${randomBytes(4).toString("hex")}${suffix}`);
      await writeFile(path, data);
      return path;
    },
    log: { debug: log("debug"), info: log("info"), warn: log("warn"), error: log("error") },
  });

  const handleInitialize = async (id: MessageId, params: InitializeParams) => {
    ctx = buildContext(params);
    try {
      handle = await def.start(ctx);
    } catch (err) {
      if (err instanceof ConfigError) {
        await peer.respondError(id, CODE_CONFIG_INVALID, err.message);
      } else {
        await peer.respondError(id, CODE_INTERNAL_ERROR, errorMessage(err));
      }
      exit(1);
      return;
    }
    await peer.respond(id, { protocol_version: PROTOCOL_VERSION });
  };

  const handleSend = async (id: MessageId, params: SendRequest) => {
    if (!ctx) {
      await peer.respondError(id, CODE_REQUEST_FAILED, "channel is not initialized");
      return;
    }
    const controller = new AbortController();
    inflight.set(id, controller);
    try {
      const result = await def.send(params, ctx, controller.signal);
      if (controller.signal.aborted) {
        await peer.respondError(id, CODE_REQUEST_CANCELLED, "request cancelled");
      } else {
        await peer.respond(id, result ?? {});
      }
    } catch (err) {
      const code = controller.signal.aborted ? CODE_REQUEST_CANCELLED : CODE_REQUEST_FAILED;
      await peer.respondError(id, code, errorMessage(err));
    } finally {
      inflight.delete(id);
    }
  };

  const handleShutdown = async (id: MessageId) => {
    await stop();
    await peer.respond(id, {});
    exit(0);
  };

  const onMessage = (msg: IncomingMessage) => {
    const { id, method, params } = msg;
    if (method === "$/cancelRequest") {
      const target = (params as { id?: MessageId } | undefined)?.id;
      if (target !== undefined) inflight.get(target)?.abort();
      return;
    }
    if (id === undefined || id === null || !method) return;
    switch (method) {
      case "initialize":
        void handleInitialize(id, params as InitializeParams);
        return;
      case "send":
        void handleSend(id, params as SendRequest);
        return;
      case "shutdown":
        void handleShutdown(id);
        return;
      default:
        void peer.respondError(id, CODE_METHOD_NOT_FOUND, `method not found: ${method}`);
    }
  };

  peer.listen(onMessage, () => {
    void stop().then(() => exit(0));
  });
}
