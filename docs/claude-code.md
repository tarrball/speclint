# Claude Code

speclint tells Claude Code to write a spec before it writes code, and checks
each spec that Claude saves. `slint init --claude` sets up these parts:

- **A skill**, `.claude/skills/spec/SKILL.md`. It tells Claude to run
  `slint context` while it plans, to write the spec, and to fix each finding
  before it starts the implementation. The hook does not run when Claude
  writes a file with a shell command, so the skill tells Claude to run
  `slint check <file>` then. It also tells Claude to run `slint check` after it
  updates the registry.
- **A hook** in `.claude/settings.json`. It makes Claude Code run
  `slint hook` after each `Write`, `Edit`, or `MultiEdit`. See
  [If the hook cannot find slint](#if-the-hook-cannot-find-slint).
- **Copilot review instructions**,
  `.github/instructions/speclint.instructions.md`. See
  [Review a pull request with Copilot](#review-a-pull-request-with-copilot).

Without `--claude`, `slint init` writes only the speclint files and the
Copilot review instructions. It does not read or change the Claude Code
settings, and it does not print the line for `CLAUDE.md`. To add the Claude
Code parts later, run `slint init --claude`.

Claude loads the skill only when it thinks that the task needs it. If no
`CLAUDE.md` tells about speclint, `init` prints a line to add to `CLAUDE.md`:

```
This project uses speclint. Before you write code for a change, use the spec skill to write a spec that passes `slint check`.
```

`init` does not change `CLAUDE.md`, because you own it.

If `.claude/settings.json` exists, `init` adds the hook to it and keeps the
other settings. It changes only the `PostToolUse` list. The other keys keep
their order and their text, and the file keeps its indent.

`init` does not change a file that exists. If the policy paths already have a
Rego file, it does not add the example rule file. To see the changes before
`init` makes them, run `slint init --claude --dry-run`. It prints each file that `init`
creates and the hook that it adds, and it writes nothing.

## What the hook does

Claude Code sends the hook a JSON event on stdin. The event gives the path of
the file that the tool wrote. `slint hook` then does these steps:

1. It ignores a file that is not a spec. A file is a spec if it is a `.yaml`
   or `.yml` file in the specs directory of a speclint project. For any other
   file, the hook exits 0 and writes nothing. If the hook cannot read the
   `.speclint.yaml` of the project, it cannot find the specs directory. Then
   it exits 1, for each `.yaml` or `.yml` file in the project.
2. It checks the spec with the spec rules. It does not report the registry
   rules, because a registry problem is not a problem in the spec.
   `slint check` and `slint doctor` report the registry.
3. It gives the result to Claude Code through its exit status:

| Result | Exit status | What Claude Code does |
|---|---|---|
| The spec has an error | 2 | It sends the findings and their fixes to Claude, with an instruction to fix the spec before it writes code. |
| The spec has only warnings | 0 | It gives the warnings to Claude as more context. It does not stop Claude. |
| The spec passes | 0 | Nothing. |
| speclint cannot check the spec, for example because a rule does not compile or `.speclint.yaml` is not valid | 1 | It shows the error to the user. Claude continues. |

## If the hook cannot find slint

The hook runs the `slint` on the PATH that Claude Code gives to hooks. That PATH
can be different from the PATH of your terminal. For example, Claude Code reads
the shell profile when it starts, so it does not see a change to PATH until
you start it again.

The hook that `slint init` writes finds `slint` before it runs `slint hook`. If `slint` is
not on PATH, the hook exits 2 with a message, and Claude Code sends the message
to Claude. Without this step, the shell exits 127. Claude Code shows that error
only to the user, and in a headless run (`claude -p`), nobody sees it.

To repair the problem, add the directory of `slint` to PATH in your shell profile,
and start Claude Code again. Then run `slint doctor` in Claude Code, so that it
checks the PATH that Claude Code has.

`slint init` does not change a hook that exists. A hook from an older `slint init`
runs only `slint hook`. To get the new hook, remove the old one and run
`slint init --claude` again.

## Feedback, not a gate

The hook runs after Claude saves the file. Thus a spec that fails stays on
disk, and the hook does not stop Claude from writing code. The message and the
skill tell Claude to fix the spec first.

A stricter setup is possible. For example, a `PreToolUse` hook can block writes
outside the specs directory, or a `Stop` hook can stop Claude from finishing
while a spec fails. speclint does not include these hooks.

## Review a pull request with Copilot

The spec tells what the change must do. The review instructions tell GitHub
Copilot code review to compare the pull request with its spec. After its usual
review, Copilot adds a "Spec deviations" section to its summary. It reports:

- a call in the spec that the code does not make, or a call in the code that
  is not in the spec or in the registry
- a new, moved, or removed service or call that the code does not agree with
- an acceptance criterion that no test checks, or a change to an item in
  `out_of_scope`
- a registry that the pull request does not update, or a spec that does not
  have `status: done`

The section is for information. Copilot does not request changes only because
of a deviation. Use the section to see how well the coding agent followed the
spec.

The instructions are a file of their own, so they do not change your
`.github/copilot-instructions.md`. Copilot reads the first 4,000 characters of
an instructions file. Keep your changes to the file short.

## Check the setup

`slint doctor` checks each part of the setup and gives a fix for each
problem. Its exit status is 1 if a check fails. Use `--format json` to get
the checks as JSON.

| Check | Fails or warns when |
|---|---|
| config | There is no `.speclint.yaml` in the directory or in a parent directory. |
| specs | The specs directory does not exist (warning). |
| data, policy | A data or policy path in `.speclint.yaml` does not exist (warning). |
| rules | A data file or a Rego file does not load, or a rule has no `custom.id`. |
| registry | No services are registered (warning), the registry is not a mapping, or a registry rule fails. If the registry records calls that break the layer rules, the check gives their number for each pair of layers, and it still passes. SV202 reports such a call when a spec changes the service. |
| claude | Passes, and shows that Claude Code is not set up, if no skill and no `slint hook` are in the project or the home directory. Then `doctor` does not do the skill, hook, and path checks. |
| skill | The skill is missing, or it no longer tells Claude to run `slint context` (warning). |
| copilot | The Copilot review instructions are missing (warning). A project that does not use Copilot can ignore this warning. |
| hook | No `PostToolUse` hook runs `slint hook` in the project, local, or user settings; a settings file sets `disableAllHooks`; or a settings file is not valid JSON. If the matcher does not match `Write`, `Edit`, and `MultiEdit`, the check warns. |
| path | `slint` is not on `PATH`, it does not run, or it is another program with the name `slint`. If the `slint` on `PATH` is not the binary that runs `doctor`, the check warns and shows the version of each. If they are the same, the check shows the path and the version of the `slint` that the hook runs. |

To repair the setup, run `slint init` in the project root, or
`slint init --claude` if you use Claude Code. It adds each missing part and
does not change the other files.
