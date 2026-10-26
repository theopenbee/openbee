# User test checklists

Send these to the user in their language, filling in `<bot>`. End every checklist by telling the user that no reply is needed when they're done, because you are watching the DB. Then run `$T watch`.

The test worker `tester` runs tasks one at a time. Don't ask it for heavy jobs such as package installs or big downloads; they stall every queued task.

## Chat channels (Telegram / Discord / Feishu / DingTalk / WeCom / WeChat)

In a direct chat with `<bot>`, send in order:

1. `hello`: a quick reply.
2. An image captioned "what is in this picture": a description of the image.
3. A small CSV file captioned "summarize this file": a summary of its content.
4. "remind me to drink water in 1 minute": a proactive reminder about a minute later.
5. "ask tester to write hello.txt containing hi and send it to me": the file arrives.
6. `/clear`: confirmation that the session was cleared.

In a test group with `<bot>`: "@<bot> hello", which should get a reply in the group.

Drop items the change doesn't touch, and add the exact scenario the branch fixes.

## Per-platform additions

- **Telegram:** for groups, Group Privacy must be off (BotFather → Bot Settings → Group Privacy → Turn off) and the bot re-added afterwards; with privacy on it doesn't see @mentions. When the change touches proactive sends, add a forum topic check: in a forum topic, ask for a reminder in 1 minute; it must arrive in the same topic.
- **Discord:** the bot must share a server with the user before DMs work, so send the invite URL first if needed. MESSAGE CONTENT INTENT must be on.
- **Feishu / DingTalk / WeCom:** in groups, @ the bot.
- **WeChat:** after the QR login, the bot sends at most 10 replies per user message. If later sends fail with `ret=-2`, ask the user to send any message to reset the quota.

## Linear (agent-driven, no user steps)

Drive it through the GraphQL API with node:

1. Create the label `openbee-e2e` on the team if it's missing.
2. Create a test issue in an allowed project and state with that label. Expect an `eyes` reaction and a reply comment.
3. Comment on the issue. Expect a reply.
4. On teardown, `issueDelete` every test issue and `issueLabelDelete` the label, then verify with a query.
