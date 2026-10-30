/** Joins the class names that are set, dropping the ones that are not. */
export function cx(...names: readonly (string | false | undefined)[]): string {
  return names.filter((name) => name !== undefined && name !== false).join(" ");
}
