# Style guide

This guide gives the rules for code and text in this repository.
Reviewers use it, and so do agents such as Claude Code.

## Writing: ASD-STE100

All text follows the main rules of
[ASD-STE100 Simplified Technical English](https://www.asd-ste100.org/) (STE).
This includes documentation, code comments, commit messages, CLI output, and the
`fix:` text in Rego rules.
STE makes text easy to read for people who do not have English as a first
language, and it makes instructions clear for agents.

We apply the rules below. We do not check each word against the STE dictionary.

### Words

- Use one word for one meaning.
- Use the same word for the same thing each time.
  A *spec* is always a spec, not a "document" in one place and a "file" in another.
- Use the simplest common word. Write "use", not "utilize". Write "start", not "initiate".
- Use technical names as they are: `slint check`, `data.project`, `SV101`.
- Do not use a noun cluster of more than three words.
  Write "the rules for the service registry", not "service registry rule set".
- Use "a", "an", and "the". Do not leave them out to make a sentence shorter.
- Do not use slang, idioms, or jokes.

### Verbs

- Use the active voice. Write "The hook checks the spec", not "The spec is checked".
- Use only simple tenses: present, past, and future.
  Write "The command creates", not "The command will be creating".
- Do not use the *-ing* form as a verb. Write "When you save the spec", not "On saving the spec".
- Use "must" for a requirement and "can" for a possibility.

### Sentences and paragraphs

- Keep an instruction to 20 words or fewer. Keep a description to 25 words or fewer.
- Give one instruction in each sentence.
- Write instructions in the imperative: "Run `slint init`."
- Put a condition before the instruction: "If the hook does not run, examine the PATH."
- Keep a paragraph to one topic and six sentences or fewer.
- Use a vertical list for a sequence of steps or a set of items.

### What to leave out

- Do not tell the reader what a tool does not need.
  Write what speclint does, not which other tools it replaces.
- Do not repeat what the code or the heading already says.
- Do not add marketing words such as "simply", "just", "powerful", or "seamless".

### Examples

| Not STE | STE |
|---|---|
| A single Go binary with OPA embedded: no conftest, Docker, or runtime to install. | speclint is one binary. It contains the policy engine. |
| Registry rules (SR\*) run once per `check` and report against the registry file. | speclint runs the registry rules one time for each `check`. It shows their findings on the registry file. |
| `// parseArgs lets flags appear before or after positional arguments.` | `// parseArgs parses flags that come before or after the positional arguments.` |

## Go

### Layout

- Put the `main` package in `cmd/slint`. Keep it small: it calls into `internal/`.
- Put all other code in packages under `internal/`. Give each package one purpose.
- Do not make a package named `util`, `common`, or `helpers`.

### Errors

- Do not ignore an error. If you must ignore one, write a comment that gives the reason.
- When you return an error, add context: `fmt.Errorf("read %s: %w", path, err)`.
- Start error text with a lowercase letter. Do not end it with a period.
- Use `errors.Is` and `errors.As`. Do not compare error strings.

### Types and values

- Use a named type and constants for a fixed set of values,
  for example a severity or an exit code. Do not use bare strings or numbers.
- Do not use package-level variables that change.
  You can use a package-level variable for a compiled `regexp` or for embedded files.

### Input and output

- Do not use `os.Stdout`, `os.Stderr`, `os.Stdin`, `os.Getwd`, or `os.Exit` outside `cmd/slint`.
  Pass an `io.Writer`, an `io.Reader`, or a directory as a parameter.
  This lets a test supply its own values.
- Use `0o644` for files and `0o755` for directories that speclint creates.

### Comments

- Write a doc comment for each package and each exported name.
  Start it with the name: `// Find returns ...`.
- Write a comment to tell *why* the code does something.
  If the code shows clearly *what* it does, do not tell it again in a comment.
- Write comments in STE (see above). End each sentence with a period.

### Tests

- Put a test in the same package as the code that it tests.
- If the cases of a test share the same steps, use a table-driven test.
- Use `t.TempDir()` for files. Do not write to the repository during a test.
- Use `testdata/` for fixed input files.
- Call `t.Helper()` in each test helper.
- A test name tells the behavior: `TestHookIgnoresFilesOutsideSpecsDir`.

### Formatting and lint

`mise run lint` must pass. `mise run fmt` formats Go code with gofumpt and goimports.
A `//nolint` directive must name the linter and give a reason:

```go
//nolint:gosec // This check must run the slint binary that is on PATH.
```

## Rego

- Use Rego v1 syntax (`import rego.v1`, `contains`, `if`).
- Give each `deny` or `warn` rule a `METADATA` block with a `title` and a `custom.id`.
  speclint does not load a rule that has no `custom.id`.
- Make each finding with `lib.finding(rego.metadata.rule(), path, msg, fix)`.
- Bind each value that can be undefined before you use it in string interpolation.
  `$"{x}"` writes an undefined `x` as `<undefined>`. It does not make the rule
  undefined, as `sprintf` does.
- Write `msg` as a statement of the problem.
  Write `fix` as one or more instructions in STE.
- `opa fmt` must not change the file. `mise run fmt` applies it.
- Test each rule in a `*_test.rego` file next to it. `go test ./internal/baseline`
  runs these tests with the real `data.dpl`. Use `with input as` and
  `with data.project as` to give each test its own spec and registry.
- Regal must not find a problem. `.regal/config.yaml` gives the reason for each rule that is off.

## Commits

- Write the subject in the imperative, with 72 characters or fewer:
  `Check the error from json.Encoder.Encode`.
- Do not end the subject with a period.
- In the body, tell *why* you made the change. The diff shows *what* changed.
- Make one logical change in each commit. Each commit must build and pass the tests.
