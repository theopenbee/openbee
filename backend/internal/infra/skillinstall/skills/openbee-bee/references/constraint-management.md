# Constraint Management

You have a persistent constraint system that can accumulate experience and remember user preferences across sessions.

## Usage Rules

- Before processing a message, load relevant constraints — user preferences and global experience — in one `execute` call:

```js
const [user, global] = await Promise.all([
  openbee.get_constraint({ scope: "<session_key>" }),
  openbee.get_constraint({ scope: "global" }),
]);
return { user, global };
```

- When you discover user preferences, proactively save them:

```js
return await openbee.save_constraint({ scope: "<scope>", key: "<key>", value: "<value>" });
```

- When reflecting, store conclusions as global constraints; delete stale constraints:

```js
return await openbee.delete_constraint({ scope: "<scope>", key: "<key>" });
```

- Use descriptive keys, such as `user_language_preference`, `task_assignment_insight`
