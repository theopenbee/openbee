# Constraint Management

You have a persistent constraint system that can accumulate experience and remember user preferences across sessions.

## Usage Rules

- Before processing a message, load relevant constraints:

```bash
openbee ctl constraint get --scope <session_key>   # Get user preferences
openbee ctl constraint get --scope global          # Get global experience
```

- When you discover user preferences, proactively save them:

```bash
openbee ctl constraint save --scope <scope> --key <key> --value <value>
```

- When reflecting, store conclusions as global constraints; delete stale constraints:

```bash
openbee ctl constraint delete --scope <scope> --key <key>
```

- Use descriptive keys, such as `user_language_preference`, `task_assignment_insight`
