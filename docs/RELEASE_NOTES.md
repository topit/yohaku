# First public preview

Yohaku is a lightweight, local-first scratchpad built with Go and MyGo Native UI.
This first public preview includes:

- Continuous, multi-language block notes, Math results and image & text notes.
- Tabs or a sidebar, command center, themes, fonts and configurable shortcuts.
- Trusted local TypeScript command plugins, without Node.js or a plugin market.
- Heynote import, autosave, per-note files, archives and recovery exports.
- macOS close-to-hide, Dock restore and save-before-quit protection.

## Download and install

- **Apple Silicon:** `Yohaku-v0.1.0-macos-arm64.zip`.
- **Intel:** `Yohaku-v0.1.0-macos-amd64.zip`.
- `SHA256SUMS` contains the archive checksums.

Quit an older Yohaku with **⌘Q**, unzip the archive, then move `Yohaku.app` to
Applications. The red window button only hides the editor; it does not quit.

These builds are **ad-hoc signed, not Developer ID signed or notarized**.
macOS may block the first launch. After verifying the download's origin, use
System Settings → Privacy & Security → Open Anyway if macOS offers it.
Do not disable Gatekeeper globally. There is no automatic updater.

## Preview limitations

- Back up important notes before upgrading. Autosave is not a substitute for a backup.
- Interactive acceptance has focused on Apple Silicon macOS; Intel builds have CI
  tests, not equivalent manual UI/IME coverage. No Linux or Windows packages ship.
- Text blocks and image & text notes remain separate editor types.
- Edits that damage a block delimiter are rejected as a whole.
- Plugins run trusted local code in-process; this is not an OS security sandbox.
- Undo history survives hiding and tab changes, but not an application restart.

See the English and Chinese READMEs for storage, recovery, build instructions,
measured performance limits and third-party acknowledgments.
