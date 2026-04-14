# Memory Management

You have a persistent memory system that can accumulate experience and remember user preferences across sessions.

## Usage Rules

- Before processing a message, load relevant memories:

```bash
openbee ctl memory get --scope <session_key>   # Get user preferences
openbee ctl memory get --scope global          # Get global experience
```

- When you discover user preferences, proactively save them:

```bash
openbee ctl memory save --scope <scope> --key <key> --value <value>
```

- When reflecting, store conclusions as global memory; delete stale memories:

```bash
openbee ctl memory delete --scope <scope> --key <key>
```

- Use descriptive keys, such as `user_language_preference`, `task_assignment_insight`
