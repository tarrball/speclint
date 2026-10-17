# Claude Code

specval tells Claude Code to write a spec before it writes code, and checks
each spec that Claude saves. `sv init` sets up two parts:

- **A skill**, `.claude/skills/spec/SKILL.md`. It tells Claude to run
  `sv context` while it plans, to write the spec, and to fix each finding
  before it starts the implementation. The hook does not run when Claude
  writes a file with a shell command, so the skill tells Claude to run
  `sv check <file>` then. It also tells Claude to run `sv check` after it
  updates the registry.
- **A hook** in `.claude/settings.json`. It makes Claude Code run
  `sv hook` after each `Write`, `Edit`, or `MultiEdit`.

If `.claude/settings.json` exists, `init` adds the hook to it and keeps the
other settings. It changes only the `PostToolUse` list. The other keys keep
their order and their text, and the file keeps its indent.

## What the hook does

Claude Code sends the hook a JSON event on stdin. The event gives the path of
the file that the tool wrote. `sv hook` then does these steps:

1. It ignores a file that is not a spec. A file is a spec if it is a `.yaml`
   or `.yml` file in the specs directory of a specval project. For any other
   file, the hook exits 0 and writes nothing. If the hook cannot read the
   `.specval.yaml` of the project, it cannot find the specs directory. Then
   it exits 1, for each `.yaml` or `.yml` file in the project.
2. It checks the spec with the spec rules. It does not report the registry
   rules, because a registry problem is not a problem in the spec.
   `sv check` and `sv doctor` report the registry.
3. It gives the result to Claude Code through its exit status:

| Result | Exit status | What Claude Code does |
|---|---|---|
| The spec has an error | 2 | It sends the findings and their fixes to Claude, with an instruction to fix the spec before it writes code. |
| The spec has only warnings | 0 | It gives the warnings to Claude as more context. It does not stop Claude. |
| The spec passes | 0 | Nothing. |
| specval cannot check the spec, for example because a rule does not compile or `.specval.yaml` is not valid | 1 | It shows the error to the user. Claude continues. |

## Feedback, not a gate

The hook runs after Claude saves the file. Thus a spec that fails stays on
disk, and the hook does not stop Claude from writing code. The message and the
skill tell Claude to fix the spec first.

A stricter setup is possible. For example, a `PreToolUse` hook can block writes
outside the specs directory, or a `Stop` hook can stop Claude from finishing
while a spec fails. specval does not include these hooks.

## Check the setup

`sv doctor` checks each part of the setup and gives a fix for each
problem. Its exit status is 1 if a check fails. Use `--format json` to get
the checks as JSON.

| Check | Fails or warns when |
|---|---|
| config | There is no `.specval.yaml` in the directory or in a parent directory. |
| specs | The specs directory does not exist (warning). |
| data, policy | A data or policy path in `.specval.yaml` does not exist (warning). |
| rules | A data file or a Rego file does not load, or a rule has no `custom.id`. |
| registry | No services are registered (warning), the registry is not a mapping, or a registry rule fails. |
| skill | The skill is missing, or it no longer tells Claude to run `sv context` (warning). |
| hook | No `PostToolUse` hook runs `sv hook` in the project, local, or user settings; a settings file sets `disableAllHooks`; or a settings file is not valid JSON. If the matcher does not match `Write`, `Edit`, and `MultiEdit`, the check warns. |
| path | `sv` is not on `PATH`, it does not run, or it is another program with the name `sv`. If the `sv` on `PATH` is not the binary that runs `doctor`, the check warns. |

To repair the setup, run `sv init` in the project root. It adds each
missing part and does not change the other files.
