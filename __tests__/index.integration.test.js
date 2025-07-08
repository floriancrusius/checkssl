const { parseArguments, readDomainsFromFile } = require('../index');

// Simple integration tests that don't rely on complex mocking
describe('index.js Integration Tests', () => {
  test('parseArguments should handle empty arguments', () => {
    const result = parseArguments([]);
    expect(Array.isArray(result)).toBe(true);
  });

  test('parseArguments should handle unknown flags', () => {
    const result = parseArguments(['--unknown']);
    expect(Array.isArray(result)).toBe(true);
  });

  test('readDomainsFromFile should handle non-existent file', () => {
    const result = readDomainsFromFile('/non/existent/file');
    expect(Array.isArray(result)).toBe(true);
  });
});
