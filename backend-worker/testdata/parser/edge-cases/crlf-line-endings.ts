// File with Windows-style CRLF line endings.
// Every line in this file uses \r\n.

export interface CrlfTest {
  name: string;
  count: number;
}

export function greet(test: CrlfTest): string {
  const message = `Hello, ${test.name}\!`;
  return message;
}

const items: CrlfTest[] = [
  { name: "alpha", count: 1 },
  { name: "beta", count: 2 },
  { name: "gamma", count: 3 },
];
