/// <reference path="./yohaku.d.ts" />

export function tidy(input: YohakuInput): string {
  return input.text.split("\n").map((line: string) => line.trimEnd()).join("\n");
}

export function upper(input: YohakuInput): string {
  return input.text.toUpperCase();
}
