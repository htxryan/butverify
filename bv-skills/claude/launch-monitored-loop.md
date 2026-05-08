---
name: launch-monitored-loop
description: Run a compound-agent infinity loop and actively monitor it, checking progress every N minutes (default interval=15). Reports epic status, current activity, and flags issues.
argument-hint: "[--target my-bead-123] [--interval 15]"
---

# Launch Monitored Loop

Actively monitors a running `ca loop` infinity loop session, checking back at regular intervals to report progress and flag issues. Your goal is to ensure the loop is progressing as expected. You are empowered to take action to get it back on track if you find the loop is stuck, errored, or needs to otherwise be steered or corrected.

## Workflow

### 1. Launch Loop (first invocation only)

Check whether a loop is already running:

```bash
screen -ls 2>/dev/null | grep beads-loop
```

- If a session is found, **skip launch** and go straight to Step 2 (this is a re-entry from a scheduled wakeup).
- If no session is found, launch a compound-agent loop using `/compound:launch-loop` against the given `--target`.

### 2. Monitor

On each check cycle, run ALL of these commands and synthesize a status report:

#### 2.1. Verify the loop is running

```bash
screen -ls 2>/dev/null | grep beads-loop || echo "NO SCREEN SESSION"
```

If no session found, report **LOOP DOWN** and stop monitoring.

#### 2.2. Check loop output for latest events

```bash
tail -20 .compound-agent/agent_logs/loop-output*.log 2>/dev/null | tail -20
```

Look for: epic completions, review phases, failures, skips, crashes.

#### 2.3. Check current epic progress

```bash
bd list --status=in_progress 2>/dev/null
bd stats 2>/dev/null
```

If `bd` is blocked (Dolt lock), note it and check process list instead.

#### 2.4. Check agent activity

```bash
# Find the current epic's agent log
tail -c 500 .compound-agent/agent_logs/loop_beads-gui-*-$(date +%Y-%m-%d)*.log 2>/dev/null | tail -10
```

#### 2.5. Check for stuck processes

```bash
ps aux | grep 'playwright' | grep -v grep | wc -l
ps aux | grep -E 'vite|dolt sql-server' | grep -v grep | wc -l
```

### 3. Prove It

Use the `/butverify:prove-it` skill to validate the end result of this loop and publish a gallery on butverify.dev.

## Status Report Format

Each check should produce a concise status like:

```
**Status: [Epic Name] -- [phase] ([N/M] done)**
- Screen: running/down
- Current: [what the agent is doing]
- Closed: X / Y total
- Issues: [any problems detected]
```

## When to Flag Issues

- **LOOP DOWN**: Screen session gone -- needs relaunch
- **STUCK**: Agent log hasn't updated in >10 minutes and no processes running
- **DOLT LOCK**: `bd` commands failing -- stale dolt sql-server process
- **TEST FAILURES**: Agent iterating on test fixes for >3 cycles
- **MEMORY**: System memory below 90% free

## Scheduling

After each check, use `ScheduleWakeup` to schedule the next one:

```
ScheduleWakeup({
  delaySeconds: interval * 60,  // Convert minutes to seconds
  reason: "Check loop -- [brief status]",
  prompt: "/launch-monitored-loop --interval N"
})
```

Use `delaySeconds` of 270 for 5-min intervals (stays within prompt cache window).

## Important Notes

- Parse the interval from the user's args (default: 15 minutes)
- Keep reports concise -- the user doesn't need every log line
- If the loop finishes (all epics done), report completion and stop monitoring
- If `bd` is blocked by Dolt lock, check processes instead -- don't keep retrying bd