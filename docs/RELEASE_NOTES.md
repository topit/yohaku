# Yohaku 0.1.1 preview

Yohaku is a lightweight, local-first scratchpad built with Go and MyGo Native UI.
This update refines the main window's paper-page presentation:

- Block actions and Math copying move into the footer, removing the empty
  toolbar row from ordinary notes. Image & text notes keep their top toolbar.
- Text and Markdown use the paper background; code and Math use a subtle tint.
  The empty canvas after the final block always returns to paper.
- Syntax colors follow each theme in both light and dark modes.
- More compact tabs retain their track. Ordinary notes no longer show a file
  icon; close buttons appear on the active or hovered tab.

The line-number setting still applies to every ordinary block. Editing, storage,
block commands and image & text entry points are unchanged.

## Download and install

- **Apple Silicon:** `Yohaku-v0.1.1-macos-arm64.zip`.
- **Intel:** `Yohaku-v0.1.1-macos-amd64.zip`.
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
