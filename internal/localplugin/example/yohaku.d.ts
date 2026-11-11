/** API v1. A copied snapshot, not a live note or filesystem handle.
 * Return replacement text synchronously. Throw to leave the note unchanged.
 * No Promise, imports, Node/npm, network, host filesystem or UI APIs.
 */
interface YohakuInput {
  readonly text: string;
  readonly language: string;
  readonly noteTitle: string;
}
