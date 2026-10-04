export function mergeModels(...groups: readonly (readonly string[])[]): string[] {
  return [...new Set(groups.flat().map(model => model.trim()).filter(Boolean))]
}

export function parseModels(input: string): string[] {
  return mergeModels(input.split(/[\n\r,;，；]+/))
}
