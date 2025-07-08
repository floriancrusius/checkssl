const { sortResults, parseDate } = require('./lib/helper');

const results = [
  { domain: 'valid.com', result: '01.01.2026' },
  { domain: 'error.com', result: '   Error  ' },
  { domain: 'invalid-date.com', result: '32.01.2025' },
  { domain: 'another-valid.com', result: '01.01.2025' },
];

console.log('Original results:', results);

console.log('\nParsing dates:');
results.forEach((r) => {
  try {
    if (!r.result.includes('Error')) {
      const parsed = parseDate(r.result);
      console.log(`${r.domain}: ${r.result} -> ${parsed}`);
    } else {
      console.log(`${r.domain}: ${r.result} -> Error (not parsed)`);
    }
  } catch (e) {
    console.log(`${r.domain}: ${r.result} -> Parse error: ${e.message}`);
  }
});

const sorted = sortResults(results);
console.log('\nSorted results:');
sorted.forEach((r, i) => {
  console.log(`${i}: ${r.domain} - ${r.result}`);
});
