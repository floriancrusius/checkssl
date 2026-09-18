const {
  isValidDomain,
  validateDomain,
  domainLengthReducer,
  sortResults,
  formatResults,
  formatResultsCSV,
  formatResultsJSON,
  separator,
  sendHelp,
  printTable,
  printInfo,
  printErrors,
  DOMAIN_REGEX,
  DATE_LOCALE,
  DATE_FORMAT_OPTIONS,
} = require('../lib/helper');

describe('Helper Functions', () => {
  describe('isValidDomain', () => {
    test('returns true for valid domains', () => {
      expect(isValidDomain('example.com')).toBe(true);
      expect(isValidDomain('subdomain.example.com')).toBe(true);
      expect(isValidDomain('test-domain.org')).toBe(true);
      expect(isValidDomain('a.co')).toBe(true);
    });

    test('returns false for invalid domains', () => {
      expect(isValidDomain('example')).toBe(false);
      expect(isValidDomain('')).toBe(false);
      expect(isValidDomain(null)).toBe(false);
      expect(isValidDomain(undefined)).toBe(false);
      expect(isValidDomain('example.')).toBe(false);
      expect(isValidDomain('.example.com')).toBe(false);
      expect(isValidDomain('example..com')).toBe(false);
      expect(isValidDomain('example.c')).toBe(false);
    });

    test('is case-insensitive', () => {
      expect(isValidDomain('EXAMPLE.COM')).toBe(true);
      expect(isValidDomain('Example.Com')).toBe(true);
    });

    test('accepts surrounding whitespace', () => {
      expect(isValidDomain('  example.com  ')).toBe(true);
      expect(isValidDomain(' \t example.com \n ')).toBe(true);
    });

    test('rejects domains over 253 chars', () => {
      expect(isValidDomain('a'.repeat(250) + '.com')).toBe(false);
    });

    test('accepts a domain exactly at 253 chars', () => {
      expect(isValidDomain('a'.repeat(249) + '.com')).toBe(true);
    });
  });

  describe('validateDomain', () => {
    test('invokes the error callback for invalid domains', () => {
      const cb = jest.fn();
      expect(validateDomain('invalid-domain-no-tld', cb)).toBe(false);
      expect(cb).toHaveBeenCalledWith(
        'Invalid domain format: invalid-domain-no-tld',
      );
    });

    test('does not invoke the callback for valid domains', () => {
      const cb = jest.fn();
      expect(validateDomain('example.com', cb)).toBe(true);
      expect(cb).not.toHaveBeenCalled();
    });

    test('tolerates a missing callback', () => {
      expect(() => validateDomain('invalid-no-tld')).not.toThrow();
      expect(validateDomain('invalid-no-tld')).toBe(false);
    });
  });

  describe('domainLengthReducer', () => {
    test('returns the maximum length seen', () => {
      expect(domainLengthReducer(0, 'example.com')).toBe(11);
      expect(domainLengthReducer(15, 'short.co')).toBe(15);
      expect(domainLengthReducer(5, 'very-long-domain.example.org')).toBe(28);
    });

    test('ignores non-string inputs', () => {
      expect(domainLengthReducer(10, null)).toBe(10);
      expect(domainLengthReducer(10, undefined)).toBe(10);
      expect(domainLengthReducer(10, 123)).toBe(10);
    });
  });

  describe('sortResults', () => {
    const results = () => [
      { domain: 'later.com', expiresAt: new Date('2026-01-01T00:00:00Z') },
      { domain: 'earlier.com', expiresAt: new Date('2025-01-01T00:00:00Z') },
      { domain: 'error.com', expiresAt: null, status: 'error' },
    ];

    test('sorts by expiresAt ascending by default; errors last', () => {
      const sorted = sortResults(results());
      expect(sorted.map((r) => r.domain)).toEqual([
        'earlier.com',
        'later.com',
        'error.com',
      ]);
    });

    test('sorts descending when requested; errors still last', () => {
      const sorted = sortResults(results(), 'desc');
      expect(sorted.map((r) => r.domain)).toEqual([
        'later.com',
        'earlier.com',
        'error.com',
      ]);
    });

    test('does not mutate the input array', () => {
      const input = results();
      const originalOrder = input.map((r) => r.domain);
      sortResults(input);
      expect(input.map((r) => r.domain)).toEqual(originalOrder);
    });

    test('handles all-error results', () => {
      expect(() =>
        sortResults([
          { domain: 'e1', expiresAt: null },
          { domain: 'e2', expiresAt: null },
        ]),
      ).not.toThrow();
    });

    test('throws for non-array input', () => {
      expect(() => sortResults('not an array')).toThrow();
      expect(() => sortResults(null)).toThrow();
    });

    test('treats invalid Date objects as errors', () => {
      const sorted = sortResults([
        { domain: 'valid.com', expiresAt: new Date('2026-01-01T00:00:00Z') },
        { domain: 'nan.com', expiresAt: new Date('invalid') },
      ]);
      expect(sorted[0].domain).toBe('valid.com');
      expect(sorted[1].domain).toBe('nan.com');
    });
  });

  describe('formatResults', () => {
    // Fix "now" so days-until-expiry is deterministic.
    const NOW = new Date(2025, 0, 1).getTime();

    test('renders padded rows with date and days-until-expiry column', () => {
      const formatted = formatResults(
        [
          { domain: 'example.com', expiresAt: new Date(2025, 1, 12) },
          { domain: 'test.org', expiresAt: new Date(2026, 1, 2) },
        ],
        15,
        NOW,
      );
      expect(formatted[0]).toBe(
        '| example.com     | 12.02.2025 |        in 42 days |',
      );
      expect(formatted[1]).toBe(
        '| test.org        | 02.02.2026 |       in 397 days |',
      );
    });

    test('enforces minimum padding of 10', () => {
      const formatted = formatResults(
        [{ domain: 'a.co', expiresAt: new Date(2025, 1, 12) }],
        3,
        NOW,
      );
      expect(formatted[0]).toBe(
        '| a.co       | 12.02.2025 |        in 42 days |',
      );
    });

    test('renders error label and N/A days for null expiresAt', () => {
      const formatted = formatResults(
        [{ domain: 'broken.com', expiresAt: null }],
        10,
        NOW,
      );
      expect(formatted[0]).toBe(
        '| broken.com |    Error   |               N/A |',
      );
    });

    test('shows "expired Xd ago" for past dates', () => {
      const formatted = formatResults(
        [{ domain: 'gone.com', expiresAt: new Date(2020, 0, 1) }],
        10,
        NOW,
      );
      expect(formatted[0]).toBe(
        '| gone.com   | 01.01.2020 | expired 1827d ago |',
      );
    });

    test('throws for non-array input', () => {
      expect(() => formatResults('not an array', 10)).toThrow();
    });
  });

  describe('formatResultsCSV', () => {
    const NOW = new Date(2025, 0, 1).getTime();

    test('produces expected header and rows with days column', () => {
      const csv = formatResultsCSV(
        [
          { domain: 'example.com', expiresAt: new Date(2025, 1, 12) },
          { domain: 'test.org', expiresAt: new Date(2025, 1, 2) },
        ],
        NOW,
      );
      const lines = csv.split('\n');
      expect(lines[0]).toBe('Domain,Expiration,DaysUntilExpiry');
      expect(lines[1]).toBe('"example.com","12.02.2025",42');
      expect(lines[2]).toBe('"test.org","02.02.2025",32');
    });

    test('emits Error and empty days for errored entries', () => {
      const csv = formatResultsCSV(
        [{ domain: 'broken.com', expiresAt: null }],
        NOW,
      );
      expect(csv.split('\n')[1]).toBe('"broken.com","Error",');
    });

    test('escapes quotes in the domain', () => {
      const csv = formatResultsCSV(
        [
          {
            domain: 'weird"name.com',
            expiresAt: new Date(2025, 0, 1),
          },
        ],
        NOW,
      );
      expect(csv.split('\n')[1]).toBe('"weird""name.com","01.01.2025",0');
    });

    test('throws for non-array input', () => {
      expect(() => formatResultsCSV('not an array')).toThrow(
        'Results must be an array',
      );
      expect(() => formatResultsCSV(null)).toThrow('Results must be an array');
    });
  });

  describe('formatResultsJSON', () => {
    const NOW = new Date(2025, 0, 1).getTime();

    test('renders domain, expiration and daysUntilExpiry', () => {
      const parsed = JSON.parse(
        formatResultsJSON(
          [
            { domain: 'example.com', expiresAt: new Date(2025, 1, 12) },
            { domain: 'test.org', expiresAt: new Date(2025, 1, 2) },
          ],
          NOW,
        ),
      );
      expect(parsed).toEqual([
        {
          domain: 'example.com',
          expiration: '12.02.2025',
          daysUntilExpiry: 42,
        },
        { domain: 'test.org', expiration: '02.02.2025', daysUntilExpiry: 32 },
      ]);
    });

    test('includes status, error and authorizationError when present', () => {
      const parsed = JSON.parse(
        formatResultsJSON(
          [
            {
              domain: 'broken.com',
              expiresAt: null,
              status: 'error',
              error: 'boom',
            },
            {
              domain: 'invalid.com',
              expiresAt: new Date(2030, 0, 1),
              status: 'invalid',
              authorizationError: 'DEPTH_ZERO_SELF_SIGNED_CERT',
            },
          ],
          NOW,
        ),
      );
      expect(parsed[0]).toEqual({
        domain: 'broken.com',
        expiration: 'Error',
        daysUntilExpiry: null,
        status: 'error',
        error: 'boom',
      });
      expect(parsed[1]).toEqual({
        domain: 'invalid.com',
        expiration: '01.01.2030',
        daysUntilExpiry: 1826,
        status: 'invalid',
        authorizationError: 'DEPTH_ZERO_SELF_SIGNED_CERT',
      });
    });

    test('handles empty array', () => {
      expect(formatResultsJSON([])).toBe('[]');
    });

    test('throws for non-array input', () => {
      expect(() => formatResultsJSON('not an array')).toThrow(
        'Results must be an array',
      );
      expect(() => formatResultsJSON(null)).toThrow('Results must be an array');
    });
  });

  describe('separator', () => {
    test('spans domain + date column + days column + borders', () => {
      // 10 (domain) + 17 (days col) + 20 (padding & borders) = 47
      expect(separator(10)).toBe('='.repeat(47));
    });

    test('enforces a minimum width of 10', () => {
      expect(separator(5)).toBe('='.repeat(47));
    });
  });

  describe('sendHelp', () => {
    test('prints usage and examples', () => {
      const spy = jest.spyOn(console, 'log').mockImplementation();
      sendHelp();
      expect(spy).toHaveBeenCalledWith('Usage: checkssl [options]');
      expect(spy).toHaveBeenCalledWith(
        '  -d, --domain <domain>  Check a specific domain',
      );
      expect(spy).toHaveBeenCalledWith(
        '  -f, --file <file>      Read domains from a file',
      );
      expect(spy).toHaveBeenCalledWith(
        '  -h, --help             Show this help message',
      );
      expect(spy).toHaveBeenCalledWith('  checkssl -d google.com');
      spy.mockRestore();
    });
  });

  describe('printTable', () => {
    test('prints separator, rows, separator', () => {
      const spy = jest.spyOn(console, 'log').mockImplementation();
      const rows = [
        '| example.com | 01.01.2025 |',
        '| test.org    | 02.02.2025 |',
      ];
      const sep = '================================';

      printTable(rows, sep);
      expect(spy).toHaveBeenNthCalledWith(1, sep);
      expect(spy).toHaveBeenNthCalledWith(2, rows[0]);
      expect(spy).toHaveBeenNthCalledWith(3, rows[1]);
      expect(spy).toHaveBeenNthCalledWith(4, sep);
      spy.mockRestore();
    });

    test('rejects non-array input', () => {
      const spy = jest.spyOn(console, 'error').mockImplementation();
      printTable('not an array', '='.repeat(20));
      expect(spy).toHaveBeenCalledWith('Error: Invalid results format');
      spy.mockRestore();
    });

    test('handles empty results', () => {
      const spy = jest.spyOn(console, 'log').mockImplementation();
      const sep = '='.repeat(20);
      printTable([], sep);
      expect(spy).toHaveBeenNthCalledWith(1, sep);
      expect(spy).toHaveBeenNthCalledWith(2, sep);
      spy.mockRestore();
    });
  });

  describe('printInfo', () => {
    test('prints the tip', () => {
      const spy = jest.spyOn(console, 'log').mockImplementation();
      printInfo();
      expect(spy).toHaveBeenCalledWith(
        '💡 Tip: Provide domains using -d option or create ~/.checkssl file',
      );
      spy.mockRestore();
    });
  });

  describe('printErrors', () => {
    test('prints error messages to stderr', () => {
      const spy = jest.spyOn(console, 'error').mockImplementation();
      printErrors(['E1', 'E2']);
      expect(spy).toHaveBeenCalledWith('❌ Errors encountered:');
      expect(spy).toHaveBeenCalledWith('   E1');
      expect(spy).toHaveBeenCalledWith('   E2');
      spy.mockRestore();
    });

    test('is a no-op for empty arrays', () => {
      const spy = jest.spyOn(console, 'error').mockImplementation();
      printErrors([]);
      expect(spy).not.toHaveBeenCalled();
      spy.mockRestore();
    });

    test('is a no-op for non-array input', () => {
      const spy = jest.spyOn(console, 'error').mockImplementation();
      printErrors('not an array');
      printErrors(null);
      printErrors(undefined);
      expect(spy).not.toHaveBeenCalled();
      spy.mockRestore();
    });
  });

  describe('DOMAIN_REGEX constant', () => {
    test('is a RegExp', () => {
      expect(DOMAIN_REGEX).toBeInstanceOf(RegExp);
    });

    test('matches valid domains', () => {
      expect(DOMAIN_REGEX.test('example.com')).toBe(true);
      expect(DOMAIN_REGEX.test('subdomain.example.org')).toBe(true);
      expect(DOMAIN_REGEX.test('test-domain.co.uk')).toBe(true);
    });

    test('does not match invalid domains', () => {
      expect(DOMAIN_REGEX.test('example')).toBe(false);
      expect(DOMAIN_REGEX.test('.example.com')).toBe(false);
      expect(DOMAIN_REGEX.test('example..com')).toBe(false);
    });
  });

  describe('DATE_LOCALE and DATE_FORMAT_OPTIONS', () => {
    test('are exported with the expected values', () => {
      expect(DATE_LOCALE).toBe('de-DE');
      expect(DATE_FORMAT_OPTIONS).toEqual({
        day: '2-digit',
        month: '2-digit',
        year: 'numeric',
      });
    });
  });
});
