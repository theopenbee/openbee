import { defineChannel, ConfigError } from "../../dist/index.js";

defineChannel({
  async start(ctx) {
    console.log("starting echo plugin");
    if (process.env.FAIL_CONFIG) {
      throw new ConfigError("token is required");
    }
    if (process.env.FAIL_START) {
      throw new Error("boom");
    }
    ctx.log.info("ready", { channel: ctx.channelId });
    return {
      stop() {
        process.stderr.write("stopped\n");
      },
    };
  },
  async send(msg, ctx, signal) {
    if (process.env.SLOW_SEND) {
      await new Promise((resolve, reject) => {
        signal.addEventListener("abort", () => reject(new Error("aborted")));
      });
    }
    let text = msg.content ?? "";
    if (process.env.SAVE_MEDIA) {
      const path = await ctx.saveMedia(Buffer.from("hello media"), "txt");
      text = "saved:" + path;
    }
    ctx.dispatch({ chat_id: "echo", sender_id: "echo", text, raw: JSON.stringify(msg) });
    return { platform_message_id: "pm-1" };
  },
});
