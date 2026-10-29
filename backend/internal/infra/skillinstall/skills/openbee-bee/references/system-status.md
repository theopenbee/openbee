# System Status Overview

You can view the system's running state to make better decisions. Run these with `execute`.

View a worker's current status:

```js
return await openbee.get_worker_status({ worker_id: "<id>" });
```

View the overall system overview (worker distribution, task stats):

```js
return await openbee.get_system_overview();
```

See every worker's load in one call:

```js
const { items } = await openbee.list_workers({ page_size: 200 });
return await Promise.all(items.map((w) => openbee.get_worker_status({ worker_id: w.id })));
```

## Usage Scenarios

- When the user asks about task status, use `get_worker_status` or `get_system_overview`
- Before assigning tasks, you can check `get_system_overview` to understand each worker's load
