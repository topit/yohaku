# Local TypeScript commands — API 1

In **Settings → Plugins**, choose **Add example**, **Trust & enable**, then save.
Select some block-note text, open `⌘K` and run **Uppercase selection**. **Tidy
current block** trims trailing whitespace from each line. Both are undoable with
`⌘Z`. You can inspect the example's source in the displayed local directory.

**Add local plugin…** selects a folder with `yohaku-plugin.json`, one `.ts` entry
and optional `yohaku.d.ts`. Adding only inspects metadata; code runs only after
explicit enable/save and command invocation. Disable or remove its registration
in settings and save. Removal never deletes source files. Saving settings cancels
an active command; quitting also cancels it instead of applying a late result.

## Contract

See [the example manifest](example/yohaku-plugin.json), [TypeScript](example/main.ts)
and [types](example/yohaku.d.ts). Each command identifies an exported function
and a `target`: `selection` (one nonempty range) or `block` (current block body).
The function receives a copied `{text, language, noteTitle}` object and must
return replacement text synchronously. Throwing leaves the note unchanged.

MoeJS compiles **erasable TypeScript** directly; interfaces, type annotations and
generics work without a transpiler process. This is not type checking. Features
requiring emitted runtime code, such as enums, are not supported. Only the entry
file executes: no imports, npm packages, Node.js, Promise-based command results,
host filesystem/network/timer/UI APIs or dynamic `eval`/`Function`.

The result applies as one normal edit only if the note, Buffer version and
selection still match. A stale result is discarded visibly. Block boundaries
remain protected: a cross-block selection can be read, but a transformation
that changes a structural delimiter is rejected entirely, never partially.
Rich/image notes and multi-selection commands are not supported in API 1.
Plugin commands appear in the command center, without custom bindings yet.

## Resource and trust boundaries

- Up to 16 registered plugins and 16 commands per plugin; API version must be 1.
- Manifest ≤16 KiB, entry ≤64 KiB, input/output text each ≤1 MiB.
- Background compilation/execution, at most one active command, one-second
  cooperative deadline and a 32 MiB VM allocation budget.
- A **fresh VM per invocation**, separate from Math; no VM or helper process
  remains registered after completion. Source is reread each time; metadata is
  refreshed on settings save and startup. No runtime cache or persistent state.

Only enable **trusted local code**. Enabling also trusts later edits to its folder.
Execution is in the app process; the VM budget/deadline is **not an OS security
sandbox, hard process-memory ceiling or guarantee against engine bugs**. No remote
installation, marketplace, privileged API or untrusted-code support is implied.
Closed-note archives, backups and note data are not supplied to plugins.
