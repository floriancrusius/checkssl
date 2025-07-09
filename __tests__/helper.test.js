const {
  isValidDomain,
  validateDomain,
  domainLengthReducer,
  parseDate,
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
    test('should return true for valid domains', () => {
      expect(isValidDomain('example.com')).toBe(true);
      expect(isValidDomain('subdomain.example.com')).toBe(true);
      expect(isValidDomain('test-domain.org')).toBe(true);
      expect(isValidDomain('a.co')).toBe(true);
    });

    test('should return false for invalid domains', () => {
      expect(isValidDomain('example')).toBe(false); // No TLD
      expect(isValidDomain('')).toBe(false);
      expect(isValidDomain(null)).toBe(false);
      expect(isValidDomain(undefined)).toBe(false);
      expect(isValidDomain('example.')).toBe(false);
      expect(isValidDomain('.example.com')).toBe(false);
      expect(isValidDomain('example..com')).toBe(false);
      expect(isValidDomain('example.c')).toBe(false); // TLD too short
    });

    test('should handle case insensitive domains', () => {
      expect(isValidDomain('EXAMPLE.COM')).toBe(true);
      expect(isValidDomain('Example.Com')).toBe(true);
    });

    test('should handle domains with whitespace', () => {
      expect(isValidDomain('  example.com  ')).toBe(true);
      expect(isValidDomain(' \t example.com \n ')).toBe(true);
    });

    test('should handle very long domains', () => {
      const longDomain = 'a'.repeat(250) + '.com';
      expect(isValidDomain(longDomain)).toBe(false);
    });

    test('should handle domains at length boundaries', () => {
      // Domain exactly at 253 characters (max length)
      const maxLengthDomain = 'a'.repeat(249) + '.com';
      expect(isValidDomain(maxLengthDomain)).toBe(true); // Should be true as it's exactly at max length

      // Domain over 253 characters (invalid)
      const overLengthDomain = 'a'.repeat(250) + '.com';
      expect(isValidDomain(overLengthDomain)).toBe(false); // Should be false due to exceeding length

      // Short valid domain
      expect(isValidDomain('a.co')).toBe(true);
    });
  });

  describe('validateDomain', () => {
    test('should call error callback for invalid domains', () => {
      const mockCallback = jest.fn();
      const result = validateDomain('invalid-domain-no-tld', mockCallback);

      expect(result).toBe(false);
      expect(mockCallback).toHaveBeenCalledWith(
        'Invalid domain format: invalid-domain-no-tld',
      );
    });

    test('should not call error callback for valid domains', () => {
      const mockCallback = jest.fn();
      const result = validateDomain('example.com', mockCallback);

      expect(result).toBe(true);
      expect(mockCallback).not.toHaveBeenCalled();
    });

    test('should handle missing callback gracefully', () => {
      expect(() => validateDomain('invalid-no-tld')).not.toThrow();
      expect(validateDomain('invalid-no-tld')).toBe(false);
    });
  });

  describe('domainLengthReducer', () => {
    test('should return the maximum length', () => {
      expect(domainLengthReducer(0, 'example.com')).toBe(11);
      expect(domainLengthReducer(15, 'short.co')).toBe(15);
      expect(domainLengthReducer(5, 'very-long-domain.example.org')).toBe(28);
    });

    test('should handle non-string inputs', () => {
      expect(domainLengthReducer(10, null)).toBe(10);
      expect(domainLengthReducer(10, undefined)).toBe(10);
      expect(domainLengthReducer(10, 123)).toBe(10);
    });
  });

  describe('parseDate', () => {
    test('should parse valid German date format', () => {
      const date = parseDate('01.01.2025');
      expect(date.getFullYear()).toBe(2025);
      expect(date.getMonth()).toBe(0); // January is 0
      expect(date.getDate()).toBe(1);
    });

    test('should parse valid American date format', () => {
      const date = parseDate('01/01/2025');
      expect(date.getFullYear()).toBe(2025);
      expect(date.getMonth()).toBe(0); // January is 0
      expect(date.getDate()).toBe(1);
    });

    test('should distinguish between German and American formats correctly', () => {
      // German format: 02.01.2025 = January 2nd
      const germanDate = parseDate('02.01.2025');
      expect(germanDate.getMonth()).toBe(0); // January
      expect(germanDate.getDate()).toBe(2);

      // American format: 02/01/2025 = February 1st
      const americanDate = parseDate('02/01/2025');
      expect(americanDate.getMonth()).toBe(1); // February
      expect(americanDate.getDate()).toBe(1);
    });

    test('should throw error for invalid date formats', () => {
      expect(() => parseDate('2025-01-01')).toThrow();
      expect(() => parseDate('invalid')).toThrow();
      expect(() => parseDate('')).toThrow();
      expect(() => parseDate(null)).toThrow();
      expect(() => parseDate('01.01')).toThrow();
      expect(() => parseDate('01/01')).toThrow();
    });

    test('should throw error for invalid date components', () => {
      expect(() => parseDate('32.01.2025')).toThrow();
      expect(() => parseDate('01.13.2025')).toThrow();
      expect(() => parseDate('abc.def.ghi')).toThrow();
    });

    test('should handle edge date values', () => {
      expect(() => parseDate('31.12.2025')).not.toThrow();
      expect(() => parseDate('01.01.1000')).not.toThrow();
      expect(() => parseDate('29.02.2024')).not.toThrow(); // Leap year
    });

    test('should reject invalid date ranges', () => {
      expect(() => parseDate('32.01.2025')).toThrow('Date out of range');
      expect(() => parseDate('01.13.2025')).toThrow('Date out of range');
      expect(() => parseDate('01.01.999')).toThrow('Date out of range');
      expect(() => parseDate('00.01.2025')).toThrow('Date out of range');
      expect(() => parseDate('01.00.2025')).toThrow('Date out of range');
    });
  });

  describe('sortResults', () => {
    const mockResults = [
      { domain: 'later.com', result: '01.01.2026' },
      { domain: 'earlier.com', result: '01.01.2025' },
      { domain: 'error.com', result: '   Error  ' },
    ];

    test('should sort results by date ascending by default', () => {
      const sorted = sortResults([...mockResults]);
      expect(sorted[0].domain).toBe('earlier.com');
      expect(sorted[1].domain).toBe('later.com');
      expect(sorted[2].domain).toBe('error.com'); // Errors should go to end
    });

    test('should sort results by date descending', () => {
      const sorted = sortResults([...mockResults], 'desc');
      expect(sorted[0].domain).toBe('later.com');
      expect(sorted[1].domain).toBe('earlier.com');
      expect(sorted[2].domain).toBe('error.com'); // Errors should go to end
    });

    test('should handle error results gracefully', () => {
      const errorResults = [
        { domain: 'error1.com', result: '   Error  ' },
        { domain: 'error2.com', result: '   Error  ' },
      ];

      expect(() => sortResults(errorResults)).not.toThrow();
    });

    test('should throw error for non-array input', () => {
      expect(() => sortResults('not an array')).toThrow();
      expect(() => sortResults(null)).toThrow();
    });
    test('should handle mixed valid and invalid date results', () => {
      const results = [
        { domain: 'valid.com', result: '01.01.2026' },
        { domain: 'error.com', result: '   Error  ' },
        { domain: 'invalid-date.com', result: '32.01.2025' },
        { domain: 'another-valid.com', result: '01.01.2025' },
      ];

      const sorted = sortResults(results);

      // Valid dates should be sorted first, then invalid dates, then explicit errors
      expect(sorted[0].domain).toBe('another-valid.com'); // 2025 comes before 2026
      expect(sorted[1].domain).toBe('valid.com'); // 2026
      expect(sorted[2].domain).toBe('invalid-date.com'); // Invalid date goes after valid dates
      expect(sorted[3].domain).toBe('error.com'); // Explicit errors go last
    });
  });

  describe('formatResults', () => {
    test('should format results correctly', () => {
      const results = [
        { domain: 'example.com', result: '01.01.2025' },
        { domain: 'test.org', result: '02.02.2026' },
      ];

      const formatted = formatResults(results, 15);
      expect(formatted[0]).toBe('| example.com     | 01.01.2025 |');
      expect(formatted[1]).toBe('| test.org        | 02.02.2026 |');
    });

    test('should handle minimum padding', () => {
      const results = [{ domain: 'a.co', result: '01.01.2025' }];
      const formatted = formatResults(results, 3);

      // Should use minimum padding of 10
      expect(formatted[0]).toBe('| a.co       | 01.01.2025 |');
    });

    test('should throw error for non-array input', () => {
      expect(() => formatResults('not an array', 10)).toThrow();
    });
  });

  describe('formatResultsCSV', () => {
    test('should format results as CSV correctly', () => {
      const results = [
        { domain: 'example.com', result: '01.01.2025' },
        { domain: 'test.org', result: '02.02.2025' },
      ];

      const csvOutput = formatResultsCSV(results);
      const lines = csvOutput.split('\n');

      expect(lines[0]).toBe('Domain,Expiration');
      expect(lines[1]).toBe('"example.com","01.01.2025"');
      expect(lines[2]).toBe('"test.org","02.02.2025"');
    });

    test('should handle CSV escaping correctly', () => {
      const results = [
        { domain: 'example,with,commas.com', result: 'Result with "quotes"' },
      ];

      const csvOutput = formatResultsCSV(results);
      const lines = csvOutput.split('\n');

      expect(lines[1]).toBe(
        '"example,with,commas.com","Result with ""quotes"""',
      );
    });

    test('should throw error for non-array input', () => {
      expect(() => formatResultsCSV('not an array')).toThrow(
        'Results must be an array',
      );
      expect(() => formatResultsCSV(null)).toThrow('Results must be an array');
    });
  });

  describe('formatResultsJSON', () => {
    test('should format results as JSON correctly', () => {
      const results = [
        { domain: 'example.com', result: '01.01.2025' },
        { domain: 'test.org', result: '02.02.2025' },
      ];

      const jsonOutput = formatResultsJSON(results);
      const parsed = JSON.parse(jsonOutput);

      expect(Array.isArray(parsed)).toBe(true);
      expect(parsed).toHaveLength(2);
      expect(parsed[0]).toEqual({
        domain: 'example.com',
        expiration: '01.01.2025',
      });
      expect(parsed[1]).toEqual({
        domain: 'test.org',
        expiration: '02.02.2025',
      });
    });

    test('should handle empty results array', () => {
      const results = [];
      const jsonOutput = formatResultsJSON(results);
      const parsed = JSON.parse(jsonOutput);

      expect(Array.isArray(parsed)).toBe(true);
      expect(parsed).toHaveLength(0);
    });

    test('should throw error for non-array input', () => {
      expect(() => formatResultsJSON('not an array')).toThrow(
        'Results must be an array',
      );
      expect(() => formatResultsJSON(null)).toThrow('Results must be an array');
    });
  });

  describe('separator', () => {
    test('should create separator with correct length', () => {
      const sep = separator(10);
      expect(sep).toBe('='.repeat(27)); // 10 + 17 for padding and borders
    });

    test('should handle minimum length', () => {
      const sep = separator(5);
      expect(sep).toBe('='.repeat(27)); // Uses minimum of 10 + 17
    });
  });

  describe('sendHelp', () => {
    test('should print help message', () => {
      const consoleSpy = jest.spyOn(console, 'log').mockImplementation();

      sendHelp();

      expect(consoleSpy).toHaveBeenCalledWith('Usage: checkssl [options]');
      expect(consoleSpy).toHaveBeenCalledWith('');
      expect(consoleSpy).toHaveBeenCalledWith('Options:');
      expect(consoleSpy).toHaveBeenCalledWith(
        '  -d <domain>      Check a specific domain',
      );
      expect(consoleSpy).toHaveBeenCalledWith(
        '  -f <file>        Read domains from a file',
      );
      expect(consoleSpy).toHaveBeenCalledWith(
        '  -s               Suppress error messages',
      );
      expect(consoleSpy).toHaveBeenCalledWith(
        '  --format <type>  Output format: table (default), csv, json',
      );
      expect(consoleSpy).toHaveBeenCalledWith(
        '  -h               Show this help message',
      );
      expect(consoleSpy).toHaveBeenCalledWith(
        '  -v, --version    Show version information',
      );
      expect(consoleSpy).toHaveBeenCalledWith('Examples:');
      expect(consoleSpy).toHaveBeenCalledWith('  checkssl -d google.com');
      expect(consoleSpy).toHaveBeenCalledWith('  checkssl -f domains.txt');
      expect(consoleSpy).toHaveBeenCalledWith(
        '  checkssl -d example.com -d another.com',
      );
      expect(consoleSpy).toHaveBeenCalledWith(
        '  checkssl -d google.com --format csv',
      );
      expect(consoleSpy).toHaveBeenCalledWith(
        '  checkssl -f domains.txt --format json',
      );
      expect(consoleSpy).toHaveBeenCalledWith('  checkssl --version');
      expect(consoleSpy).toHaveBeenCalledWith(
        '  checkssl -d example.com -d another.com',
      );
      expect(consoleSpy).toHaveBeenCalledWith(
        'If no options are provided, checkssl will look for ~/.checkssl',
      );

      consoleSpy.mockRestore();
    });
  });

  describe('printTable', () => {
    test('should print formatted table', () => {
      const consoleSpy = jest.spyOn(console, 'log').mockImplementation();
      const results = [
        '| example.com | 01.01.2025 |',
        '| test.org    | 02.02.2025 |',
      ];
      const separator = '================================';

      printTable(results, separator);

      expect(consoleSpy).toHaveBeenCalledWith(separator);
      expect(consoleSpy).toHaveBeenCalledWith('| example.com | 01.01.2025 |');
      expect(consoleSpy).toHaveBeenCalledWith('| test.org    | 02.02.2025 |');
      expect(consoleSpy).toHaveBeenCalledWith(separator);

      consoleSpy.mockRestore();
    });

    test('should handle invalid results gracefully', () => {
      const consoleErrorSpy = jest.spyOn(console, 'error').mockImplementation();

      printTable('not an array', '='.repeat(20));

      expect(consoleErrorSpy).toHaveBeenCalledWith(
        'Error: Invalid results format',
      );

      consoleErrorSpy.mockRestore();
    });

    test('should handle empty results', () => {
      const consoleSpy = jest.spyOn(console, 'log').mockImplementation();
      const separator = '='.repeat(20);

      printTable([], separator);

      expect(consoleSpy).toHaveBeenCalledWith(separator);
      expect(consoleSpy).toHaveBeenCalledWith(separator);

      consoleSpy.mockRestore();
    });
  });

  describe('printInfo', () => {
    test('should print informational message', () => {
      const consoleSpy = jest.spyOn(console, 'log').mockImplementation();

      printInfo();

      expect(consoleSpy).toHaveBeenCalledWith('');
      expect(consoleSpy).toHaveBeenCalledWith(
        '💡 Tip: Provide domains using -d option or create ~/.checkssl file',
      );
      expect(consoleSpy).toHaveBeenCalledWith(
        '   Example: checkssl -d example.com',
      );

      consoleSpy.mockRestore();
    });
  });

  describe('printErrors', () => {
    test('should print error messages', () => {
      const consoleSpy = jest.spyOn(console, 'log').mockImplementation();
      const errors = [
        'Error 1: Domain not found',
        'Error 2: Connection timeout',
      ];

      printErrors(errors);

      expect(consoleSpy).toHaveBeenCalledWith('');
      expect(consoleSpy).toHaveBeenCalledWith('❌ Errors encountered:');
      expect(consoleSpy).toHaveBeenCalledWith('');
      expect(consoleSpy).toHaveBeenCalledWith('   Error 1: Domain not found');
      expect(consoleSpy).toHaveBeenCalledWith('   Error 2: Connection timeout');

      consoleSpy.mockRestore();
    });

    test('should not print anything for empty errors array', () => {
      const consoleSpy = jest.spyOn(console, 'log').mockImplementation();

      printErrors([]);

      expect(consoleSpy).not.toHaveBeenCalled();

      consoleSpy.mockRestore();
    });

    test('should not print anything for non-array input', () => {
      const consoleSpy = jest.spyOn(console, 'log').mockImplementation();

      printErrors('not an array');
      printErrors(null);
      printErrors(undefined);

      expect(consoleSpy).not.toHaveBeenCalled();

      consoleSpy.mockRestore();
    });
  });

  describe('DOMAIN_REGEX constant', () => {
    test('should be exported and accessible', () => {
      expect(DOMAIN_REGEX).toBeDefined();
      expect(DOMAIN_REGEX).toBeInstanceOf(RegExp);
    });

    test('should match valid domains', () => {
      expect(DOMAIN_REGEX.test('example.com')).toBe(true);
      expect(DOMAIN_REGEX.test('subdomain.example.org')).toBe(true);
      expect(DOMAIN_REGEX.test('test-domain.co.uk')).toBe(true);
    });

    test('should not match invalid domains', () => {
      expect(DOMAIN_REGEX.test('example')).toBe(false);
      expect(DOMAIN_REGEX.test('.example.com')).toBe(false);
      expect(DOMAIN_REGEX.test('example..com')).toBe(false);
    });
  });

  describe('DATE_LOCALE and DATE_FORMAT_OPTIONS', () => {
    test('should be exported and have correct values', () => {
      expect(DATE_LOCALE).toBe('de-DE');
      expect(DATE_FORMAT_OPTIONS).toEqual({
        day: '2-digit',
        month: '2-digit',
        year: 'numeric',
      });
    });
  });
});
