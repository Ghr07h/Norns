# Status View

## Task

> `task_00000001-0000-4000-8000-000000000001`

**Purpose**

> Investigate

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

## Status

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
- **Task_purpose_relevant:** `true`
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

## Todo_explorations

| id | title |
| --- | --- |

## Running_explorations

| id | title | duration_seconds |
| --- | --- | --- |
| `exp_1` | `Inspect` | `0` |

## Recent_top10_derived_cognitions

| id | title | created_at |
| --- | --- | --- |
| `cog_4` | `Finding` | `2026-10-12T08:30:00Z` |

## Generated_at

`2026-10-12T08:30:00Z`

