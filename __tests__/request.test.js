const getCertificate = require('../lib/request');
const https = require('https');

// Mock the https module
jest.mock('https');

describe('getCertificate', () => {
  let mockRequest;
  let mockSocket;

  beforeEach(() => {
    mockSocket = {
      getPeerCertificate: jest.fn(),
      on: jest.fn(),
    };

    mockRequest = {
      on: jest.fn(),
      end: jest.fn(),
      destroy: jest.fn(),
    };

    https.request.mockReturnValue(mockRequest);
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  test('should resolve with formatted date for valid certificate', async () => {
    const mockCertificate = {
      valid_to: 'Jan 1 2030 00:00:00 GMT', // Future date
    };

    mockSocket.getPeerCertificate.mockReturnValue(mockCertificate);

    // Mock the response
    const mockResponse = { socket: mockSocket };

    // Simulate successful request
    https.request.mockImplementation((options, callback) => {
      process.nextTick(() => callback(mockResponse));
      return mockRequest;
    });

    const result = await getCertificate('example.com');
    expect(result).toBe('01.01.2030');
  });

  test('should reject when domain is not a string', async () => {
    await expect(getCertificate(null)).rejects.toThrow(
      'Domain must be a non-empty string',
    );
    await expect(getCertificate('')).rejects.toThrow(
      'Domain must be a non-empty string',
    );
    await expect(getCertificate(123)).rejects.toThrow(
      'Domain must be a non-empty string',
    );
  });

  test('should reject when no certificate is found', async () => {
    mockSocket.getPeerCertificate.mockReturnValue(null);

    const mockResponse = { socket: mockSocket };

    https.request.mockImplementation((options, callback) => {
      process.nextTick(() => callback(mockResponse));
      return mockRequest;
    });

    await expect(getCertificate('example.com')).rejects.toThrow(
      'No valid certificate found for example.com',
    );
  });

  test('should reject when certificate is already expired', async () => {
    const mockCertificate = {
      valid_to: 'Jan 1 2020 00:00:00 GMT', // Past date
    };

    mockSocket.getPeerCertificate.mockReturnValue(mockCertificate);

    const mockResponse = { socket: mockSocket };

    https.request.mockImplementation((options, callback) => {
      process.nextTick(() => callback(mockResponse));
      return mockRequest;
    });

    await expect(getCertificate('example.com')).rejects.toThrow(
      'Certificate for example.com has already expired',
    );
  });

  test('should reject on request error', async () => {
    https.request.mockImplementation(() => {
      process.nextTick(() => {
        const errorCallback = mockRequest.on.mock.calls.find(
          (call) => call[0] === 'error',
        )[1];
        errorCallback(new Error('Connection failed'));
      });
      return mockRequest;
    });

    await expect(getCertificate('example.com')).rejects.toThrow(
      'Connection failed for example.com: Connection failed',
    );
  });

  test('should reject on timeout', async () => {
    https.request.mockImplementation(() => {
      process.nextTick(() => {
        const timeoutCallback = mockRequest.on.mock.calls.find(
          (call) => call[0] === 'timeout',
        )[1];
        timeoutCallback();
      });
      return mockRequest;
    });

    await expect(getCertificate('example.com')).rejects.toThrow(
      'Request timeout for example.com',
    );
  });

  test('should use custom timeout', () => {
    const customTimeout = 10000;
    getCertificate('example.com', customTimeout);

    expect(https.request).toHaveBeenCalledWith(
      expect.objectContaining({
        host: 'example.com',
        port: 443,
        method: 'GET',
        timeout: customTimeout,
        rejectUnauthorized: true,
      }),
      expect.any(Function),
    );
  });

  test('should handle socket errors', async () => {
    https.request.mockImplementation(() => {
      process.nextTick(() => {
        const socketCallback = mockRequest.on.mock.calls.find(
          (call) => call[0] === 'socket',
        )[1];
        const mockSocketWithError = {
          ...mockSocket,
          on: jest.fn((event, callback) => {
            if (event === 'error') {
              process.nextTick(() => callback(new Error('Socket error')));
            }
          }),
        };
        socketCallback(mockSocketWithError);
      });
      return mockRequest;
    });

    await expect(getCertificate('example.com')).rejects.toThrow(
      'Socket error for example.com: Socket error',
    );
  });

  test('should handle certificate processing errors', async () => {
    const mockCertificate = {
      valid_to: 'invalid-date-format',
    };

    mockSocket.getPeerCertificate.mockReturnValue(mockCertificate);

    const mockResponse = { socket: mockSocket };

    https.request.mockImplementation((options, callback) => {
      process.nextTick(() => callback(mockResponse));
      return mockRequest;
    });

    await expect(getCertificate('example.com')).rejects.toThrow(
      'Failed to process certificate for example.com',
    );
  });

  test('should handle certificate without valid_to field', async () => {
    const mockCertificate = {
      // Missing valid_to field
      subject: { CN: 'example.com' },
    };

    mockSocket.getPeerCertificate.mockReturnValue(mockCertificate);

    const mockResponse = { socket: mockSocket };

    https.request.mockImplementation((options, callback) => {
      process.nextTick(() => callback(mockResponse));
      return mockRequest;
    });

    await expect(getCertificate('example.com')).rejects.toThrow(
      'No valid certificate found for example.com',
    );
  });

  test('should register socket event handler', () => {
    // Start the request
    getCertificate('example.com');

    // Verify that the request setup registers socket event handlers
    expect(mockRequest.on).toHaveBeenCalledWith('socket', expect.any(Function));
  });

  test('should use default timeout of 5000ms', () => {
    getCertificate('example.com');

    expect(https.request).toHaveBeenCalledWith(
      expect.objectContaining({
        timeout: 5000,
      }),
      expect.any(Function),
    );
  });

  test('should set rejectUnauthorized to true for security', () => {
    getCertificate('example.com');

    expect(https.request).toHaveBeenCalledWith(
      expect.objectContaining({
        rejectUnauthorized: true,
      }),
      expect.any(Function),
    );
  });

  test('should register timeout event handler', () => {
    getCertificate('example.com');

    expect(mockRequest.on).toHaveBeenCalledWith(
      'timeout',
      expect.any(Function),
    );
  });

  test('should call req.end() to initiate request', () => {
    getCertificate('example.com');

    expect(mockRequest.end).toHaveBeenCalled();
  });

  test('should handle empty domain string', async () => {
    await expect(getCertificate('')).rejects.toThrow(
      'Domain must be a non-empty string',
    );
  });

  test('should handle whitespace-only domain', async () => {
    await expect(getCertificate('   ')).rejects.toThrow(
      'Domain must be a non-empty string',
    );
  });

  test('should format date correctly for different locales', async () => {
    const mockCertificate = {
      valid_to: 'Dec 25 2030 12:00:00 GMT',
    };

    mockSocket.getPeerCertificate.mockReturnValue(mockCertificate);

    const mockResponse = { socket: mockSocket };

    https.request.mockImplementation((options, callback) => {
      process.nextTick(() => callback(mockResponse));
      return mockRequest;
    });

    const result = await getCertificate('example.com');
    // Should be German date format (DD.MM.YYYY)
    expect(result).toMatch(/^\d{2}\.\d{2}\.\d{4}$/);
    expect(result).toBe('25.12.2030'); // German date format
  });
});
