// UTF-8 BOM file — the first 3 bytes are the BOM marker.
// This tests that the parser strips the BOM before parsing.

export interface BomTest {
  id: string;
  value: number;
}

export function processBom(input: BomTest): string {
  return `${input.id}: ${input.value}`;
}

const DEFAULT_VALUE: BomTest = { id: "bom-test", value: 42 };
