# Trace View

## Current_exploration

- **Id:** `exp_1`
- **Title:** `Inspect`

**Content**

> Inspect evidence

## Graph

### Cognitions

```text
cog_1: Subject information
cog_4: Finding <- exp_1
```

### Explorations

```text
exp_1: (running) Inspect <- ['cog_1', 'cog_2']
```

## Parent_task

**Purpose**

> Investigate

**Note**

> Complete only current\_exploration. Use the parent Task's purpose only as context for understanding this exploration. Do not attempt to complete the entire parent Task.

## Trace

### Task-Init Cognitions

#### `cog_1` - Subject information

- **Created_at:** `2026-10-12T08:30:00Z`
- **Origin_ref:** `task_00000001-0000-4000-8000-000000000001`
- **Origin:** `task`

**Content**

> Subject information

### `exp_1` - Inspect

- **Status:** `running`
- **Sources:** `cog_1`, `cog_2`
- **Created_at:** `2026-10-12T08:30:00Z`

**Content**

> Inspect evidence

#### `cog_4` - Finding

- **Created_at:** `2026-10-12T08:30:00Z`
- **Origin_ref:** `exp_1`
- **Origin:** `exploration`

**Content**

> Evidence found

#### `cog_3` - User hint

- **Created_at:** `2026-10-12T08:30:00Z`
- **Origin:** `user`

**Content**

> Check the original evidence

## Global_hints

### `cog_2` - Global context

- **Created_at:** `2026-10-12T08:30:00Z`
- **Origin:** `user`

**Content**

> Shared context

## Running_explorations

| id | title | duration_seconds | explorer_id |
| --- | --- | --- | --- |
| `exp_1` | `Inspect` | `0` | `executor-1` |

## Generated_at

`2026-10-12T08:30:00Z`

