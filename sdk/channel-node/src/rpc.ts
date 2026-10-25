import { createInterface } from "node:readline";

export const PROTOCOL_VERSION = 1;
export const CODE_CONFIG_INVALID = -32001;
export const CODE_METHOD_NOT_FOUND = -32601;
export const CODE_INTERNAL_ERROR = -32603;
export const CODE_REQUEST_FAILED = -32000;
export const CODE_REQUEST_CANCELLED = -32800;

export type MessageId = number | string;

export interface IncomingMessage {
  id?: MessageId;
  method?: string;
  params?: unknown;
}

export class RpcError extends Error {
  constructor(
    readonly code: number,
    message: string,
  ) {
    super(message);
  }
}

export class Peer {
  private queue: Promise<void> = Promise.resolve();

  constructor(
    private readonly input: NodeJS.ReadableStream,
    private readonly output: NodeJS.WritableStream,
  ) {}

  listen(onMessage: (msg: IncomingMessage) => void, onClose: () => void): void {
    const rl = createInterface({ input: this.input, crlfDelay: Infinity });
    rl.on("line", (line) => {
      const trimmed = line.trim();
      if (!trimmed) return;
      let msg: IncomingMessage;
      try {
        msg = JSON.parse(trimmed);
      } catch {
        return;
      }
      if (msg && typeof msg === "object") onMessage(msg);
    });
    rl.on("close", onClose);
  }

  notify(method: string, params?: unknown): Promise<void> {
    return this.write({ jsonrpc: "2.0", method, params });
  }

  respond(id: MessageId, result: unknown): Promise<void> {
    return this.write({ jsonrpc: "2.0", id, result: result ?? {} });
  }

  respondError(id: MessageId, code: number, message: string): Promise<void> {
    return this.write({ jsonrpc: "2.0", id, error: { code, message } });
  }

  private write(obj: unknown): Promise<void> {
    const line = JSON.stringify(obj) + "\n";
    this.queue = this.queue.then(
      () =>
        new Promise<void>((resolve) => {
          this.output.write(line, () => resolve());
        }),
    );
    return this.queue;
  }
}
