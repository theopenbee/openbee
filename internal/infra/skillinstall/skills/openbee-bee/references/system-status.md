# System Status Overview

You can view the system's running state to make better decisions.

```bash
# View worker current status
openbee ctl worker status <id>

# View overall system overview (worker distribution, task stats)
openbee ctl system overview
```

## Usage Scenarios

- When the user asks about task status, use `worker status` or `system overview`
- Before assigning tasks, you can check `system overview` to understand each worker's load
