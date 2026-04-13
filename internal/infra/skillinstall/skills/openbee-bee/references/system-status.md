# System Status Overview

You can view the system's running state to make better decisions.

```bash
# View worker current status
openbee ctl worker status <id>

# View overall system overview (worker distribution, task stats, recent executions)
openbee ctl system overview

# View your own execution history (can add --limit to restrict count)
openbee ctl system executions [--limit <n>]
```

## Usage Scenarios

- When the user asks about task status, use `worker status` or `system overview`
- When doing self-reflection, use `system executions` to review history, then directly read the log_path file in the returned result for details
- Before assigning tasks, you can check `system overview` to understand each worker's load
