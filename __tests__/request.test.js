const getCertificate = require('../lib/request');
const tls = require('tls');
const { EventEmitter } = require('events');

jest.mock('tls');

/**
 * Build a fake TLS socket with configurable behavior.
 * - certificate: value returned by getPeerCertificate()
 * - authorized: socket.authorized boolean
 * - authorizationError: socket.authorizationError value
 * - onConnect: 'immediate' (invoke callback next tick), or omitted
 */
const buildMockSocket = ({
  certificate,
  authorized = true,
  authorizationError = null,
} = {}) => {
  const socket = new EventEmitter();
  socket.authorized = authorized;
  socket.authorizationError = authorizationError;
  socket.getPeerCertificate = jest.fn().mockReturnValue(certificate);
  socket.end = jest.fn();
  socket.destroy = jest.fn();
  return socket;
};

describe('getCertificate', () => {
  afterEach(() => {
    jest.clearAllMocks();
  });

  test('resolves with cert info for a valid, authorized certificate', async () => {
    const socket = buildMockSocket({
      certificate: {
        valid_from: 'Jan 1 2024 00:00:00 GMT',
        valid_to: 'Jan 1 2030 00:00:00 GMT',
        issuer: { CN: 'Test CA' },
        subject: { CN: 'example.com' },
        subjectaltname: 'DNS:example.com',
      },
      authorized: true,
    });

    tls.connect.mockImplementation((options, onConnect) => {
      process.nextTick(onConnect);
      return socket;
    });

    const info = await getCertificate('example.com');

    expect(tls.connect).toHaveBeenCalledWith(
      expect.objectContaining({
        host: 'example.com',
        port: 443,
        servername: 'example.com',
        rejectUnauthorized: false,
        timeout: 5000,
      }),
      expect.any(Function),
    );
    expect(info.validTo).toEqual(new Date('Jan 1 2030 00:00:00 GMT'));
    expect(info.validFrom).toEqual(new Date('Jan 1 2024 00:00:00 GMT'));
    expect(info.authorized).toBe(true);
    expect(info.authorizationError).toBeNull();
    expect(info.subjectAltName).toBe('DNS:example.com');
    expect(socket.end).toHaveBeenCalled();
  });

  test('resolves for expired certificate instead of rejecting', async () => {
    const socket = buildMockSocket({
      certificate: { valid_to: 'Jan 1 2020 00:00:00 GMT' },
      authorized: false,
      authorizationError: 'CERT_HAS_EXPIRED',
    });

    tls.connect.mockImplementation((options, onConnect) => {
      process.nextTick(onConnect);
      return socket;
    });

    const info = await getCertificate('expired.example');
    expect(info.validTo).toEqual(new Date('Jan 1 2020 00:00:00 GMT'));
    expect(info.authorized).toBe(false);
    expect(info.authorizationError).toBe('CERT_HAS_EXPIRED');
  });

  test('resolves for self-signed certificate instead of rejecting', async () => {
    const socket = buildMockSocket({
      certificate: { valid_to: 'Jan 1 2030 00:00:00 GMT' },
      authorized: false,
      authorizationError: 'DEPTH_ZERO_SELF_SIGNED_CERT',
    });

    tls.connect.mockImplementation((options, onConnect) => {
      process.nextTick(onConnect);
      return socket;
    });

    const info = await getCertificate('selfsigned.example');
    expect(info.authorized).toBe(false);
    expect(info.authorizationError).toBe('DEPTH_ZERO_SELF_SIGNED_CERT');
  });

  test('rejects when domain is not a non-empty string', async () => {
    await expect(getCertificate(null)).rejects.toThrow(
      'Domain must be a non-empty string',
    );
    await expect(getCertificate('')).rejects.toThrow(
      'Domain must be a non-empty string',
    );
    await expect(getCertificate(123)).rejects.toThrow(
      'Domain must be a non-empty string',
    );
    await expect(getCertificate('   ')).rejects.toThrow(
      'Domain must be a non-empty string',
    );
    expect(tls.connect).not.toHaveBeenCalled();
  });

  test('rejects when the peer returns no certificate', async () => {
    const socket = buildMockSocket({ certificate: {} });

    tls.connect.mockImplementation((options, onConnect) => {
      process.nextTick(onConnect);
      return socket;
    });

    await expect(getCertificate('example.com')).rejects.toThrow(
      'No valid certificate found for example.com',
    );
  });

  test('rejects when cert has unparseable valid_to', async () => {
    const socket = buildMockSocket({
      certificate: { valid_to: 'not-a-date' },
    });

    tls.connect.mockImplementation((options, onConnect) => {
      process.nextTick(onConnect);
      return socket;
    });

    await expect(getCertificate('example.com')).rejects.toThrow(
      'Invalid certificate date for example.com: not-a-date',
    );
  });

  test('rejects on socket error', async () => {
    const socket = buildMockSocket({});
    tls.connect.mockImplementation(() => {
      process.nextTick(() => socket.emit('error', new Error('ECONNREFUSED')));
      return socket;
    });

    await expect(getCertificate('example.com')).rejects.toThrow(
      'Connection failed for example.com: ECONNREFUSED',
    );
  });

  test('rejects on timeout and destroys the socket', async () => {
    const socket = buildMockSocket({});
    tls.connect.mockImplementation(() => {
      process.nextTick(() => socket.emit('timeout'));
      return socket;
    });

    await expect(getCertificate('example.com')).rejects.toThrow(
      'Request timeout for example.com',
    );
    expect(socket.destroy).toHaveBeenCalled();
  });

  test('respects custom timeout and port', () => {
    const socket = buildMockSocket({
      certificate: { valid_to: 'Jan 1 2030 00:00:00 GMT' },
    });
    tls.connect.mockReturnValue(socket);

    getCertificate('example.com', { timeout: 10000, port: 8443 });

    expect(tls.connect).toHaveBeenCalledWith(
      expect.objectContaining({ timeout: 10000, port: 8443 }),
      expect.any(Function),
    );
  });

  test('trims the servername', () => {
    const socket = buildMockSocket({
      certificate: { valid_to: 'Jan 1 2030 00:00:00 GMT' },
    });
    tls.connect.mockReturnValue(socket);

    getCertificate('  example.com  ');

    expect(tls.connect).toHaveBeenCalledWith(
      expect.objectContaining({
        host: 'example.com',
        servername: 'example.com',
      }),
      expect.any(Function),
    );
  });

  test('only settles once even if multiple events fire', async () => {
    const socket = buildMockSocket({
      certificate: { valid_to: 'Jan 1 2030 00:00:00 GMT' },
    });

    tls.connect.mockImplementation((options, onConnect) => {
      process.nextTick(() => {
        onConnect();
        // A late error must not throw an unhandled rejection.
        socket.emit('error', new Error('post-connect error'));
      });
      return socket;
    });

    const info = await getCertificate('example.com');
    expect(info.validTo).toEqual(new Date('Jan 1 2030 00:00:00 GMT'));
  });
});
