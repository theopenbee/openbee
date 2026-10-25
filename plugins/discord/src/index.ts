import { defineChannel } from "@theopenbee/channel-sdk";

import { createDiscordChannel } from "./discord.js";

defineChannel(createDiscordChannel());
