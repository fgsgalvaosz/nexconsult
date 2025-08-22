/**
 * ErrorHandlerService - Centralized error handling with retry strategies and structured logging
 * Handles all error scenarios with intelligent retry logic and comprehensive error reporting
 */
class ErrorHandlerService {
    constructor() {
        this.errorCounts = new Map();
        this.retryStrategies = new Map();
        this.isDevelopment = process.env.NODE_ENV !== 'production';
        
        // Initialize default retry strategies
        this.initializeRetryStrategies();
    }

    /**
     * Initialize default retry strategies for different error types
     */
    initializeRetryStrategies() {
        // Captcha-related errors
        this.retryStrategies.set('CAPTCHA_ERROR', {
            maxRetries: 3,
            baseDelay: 2000,
            backoffMultiplier: 1.5,
            shouldRetry: (error, attempt) => attempt < 3
        });

        // Network/timeout errors
        this.retryStrategies.set('NETWORK_ERROR', {
            maxRetries: 5,
            baseDelay: 1000,
            backoffMultiplier: 2,
            shouldRetry: (error, attempt) => attempt < 5
        });

        // Page navigation errors
        this.retryStrategies.set('NAVIGATION_ERROR', {
            maxRetries: 3,
            baseDelay: 3000,
            backoffMultiplier: 1.2,
            shouldRetry: (error, attempt) => attempt < 3
        });

        // Extraction errors
        this.retryStrategies.set('EXTRACTION_ERROR', {
            maxRetries: 2,
            baseDelay: 1000,
            backoffMultiplier: 1,
            shouldRetry: (error, attempt) => attempt < 2
        });

        // Generic errors
        this.retryStrategies.set('GENERIC_ERROR', {
            maxRetries: 2,
            baseDelay: 1500,
            backoffMultiplier: 1.5,
            shouldRetry: (error, attempt) => attempt < 2
        });
    }

    /**
     * Classify error type based on error message and context
     * @param {Error} error - The error object
     * @param {string} context - Context where error occurred
     * @returns {string} Error type classification
     */
    classifyError(error, context = '') {
        const message = error.message.toLowerCase();
        const stack = error.stack?.toLowerCase() || '';

        // Captcha-related errors
        if (message.includes('captcha') || 
            message.includes('esclarecimentos adicionais') ||
            message.includes('hcaptcha') ||
            context.includes('captcha')) {
            return 'CAPTCHA_ERROR';
        }

        // Network/timeout errors
        if (message.includes('timeout') ||
            message.includes('navigation') ||
            message.includes('net::') ||
            message.includes('connection') ||
            stack.includes('timeout')) {
            return 'NETWORK_ERROR';
        }

        // Page navigation errors
        if (message.includes('page') ||
            message.includes('navigation') ||
            message.includes('goto') ||
            context.includes('navigation')) {
            return 'NAVIGATION_ERROR';
        }

        // Extraction errors
        if (message.includes('extract') ||
            message.includes('jsdom') ||
            message.includes('parsing') ||
            context.includes('extraction')) {
            return 'EXTRACTION_ERROR';
        }

        return 'GENERIC_ERROR';
    }

    /**
     * Handle error with appropriate retry strategy
     * @param {Error} error - The error object
     * @param {string} context - Context where error occurred
     * @param {number} attempt - Current attempt number
     * @param {Object} metadata - Additional metadata
     * @returns {Object} Error handling result
     */
    async handleError(error, context = '', attempt = 1, metadata = {}) {
        const errorType = this.classifyError(error, context);
        const strategy = this.retryStrategies.get(errorType);
        
        // Log structured error
        this.logError(error, context, attempt, errorType, metadata);
        
        // Update error counts
        this.updateErrorCounts(errorType);
        
        // Determine if should retry
        const shouldRetry = strategy.shouldRetry(error, attempt);
        const delay = this.calculateDelay(strategy, attempt);
        
        return {
            errorType,
            shouldRetry,
            delay,
            attempt,
            maxRetries: strategy.maxRetries,
            recommendation: this.getErrorRecommendation(errorType, attempt, metadata)
        };
    }

    /**
     * Calculate delay for retry based on strategy and attempt
     * @param {Object} strategy - Retry strategy
     * @param {number} attempt - Current attempt number
     * @returns {number} Delay in milliseconds
     */
    calculateDelay(strategy, attempt) {
        return Math.floor(strategy.baseDelay * Math.pow(strategy.backoffMultiplier, attempt - 1));
    }

    /**
     * Log structured error information
     * @param {Error} error - The error object
     * @param {string} context - Context where error occurred
     * @param {number} attempt - Current attempt number
     * @param {string} errorType - Classified error type
     * @param {Object} metadata - Additional metadata
     */
    logError(error, context, attempt, errorType, metadata) {
        const errorInfo = {
            message: error.message,
            type: errorType,
            context,
            attempt,
            timestamp: new Date().toISOString(),
            stack: this.isDevelopment ? error.stack : undefined,
            metadata
        };

        if (attempt === 1) {
            console.error(`🚨 ErrorHandler: ${errorType} in ${context}:`, errorInfo);
        } else {
            console.warn(`🔄 ErrorHandler: Retry ${attempt} for ${errorType} in ${context}:`, errorInfo);
        }
    }

    /**
     * Update error counts for monitoring
     * @param {string} errorType - Error type
     */
    updateErrorCounts(errorType) {
        const current = this.errorCounts.get(errorType) || 0;
        this.errorCounts.set(errorType, current + 1);
    }

    /**
     * Get error-specific recommendations
     * @param {string} errorType - Error type
     * @param {number} attempt - Current attempt number
     * @param {Object} metadata - Additional metadata
     * @returns {string} Recommendation message
     */
    getErrorRecommendation(errorType, attempt, metadata) {
        switch (errorType) {
            case 'CAPTCHA_ERROR':
                if (attempt === 1) {
                    return 'Try refreshing page and solving captcha again';
                } else if (attempt === 2) {
                    return 'Consider using different captcha solving strategy';
                } else {
                    return 'Captcha solving failed multiple times, may need manual intervention';
                }

            case 'NETWORK_ERROR':
                if (attempt < 3) {
                    return 'Network issue detected, retrying with exponential backoff';
                } else {
                    return 'Persistent network issues, check connection and server status';
                }

            case 'NAVIGATION_ERROR':
                if (attempt === 1) {
                    return 'Page navigation failed, retrying with fresh browser context';
                } else {
                    return 'Multiple navigation failures, target site may be down';
                }

            case 'EXTRACTION_ERROR':
                return 'Data extraction failed, page structure may have changed';

            default:
                return 'Generic error occurred, check logs for details';
        }
    }

    /**
     * Get error statistics
     * @returns {Object} Error statistics
     */
    getErrorStats() {
        const stats = {};
        for (const [errorType, count] of this.errorCounts.entries()) {
            stats[errorType] = count;
        }
        
        return {
            totalErrors: Array.from(this.errorCounts.values()).reduce((sum, count) => sum + count, 0),
            errorsByType: stats,
            timestamp: new Date().toISOString()
        };
    }

    /**
     * Reset error counts
     */
    resetErrorCounts() {
        this.errorCounts.clear();
        console.log('ErrorHandler: Error counts reset');
    }

    /**
     * Check if error rate is too high for a specific type
     * @param {string} errorType - Error type to check
     * @param {number} threshold - Error count threshold
     * @returns {boolean} True if error rate is too high
     */
    isErrorRateTooHigh(errorType, threshold = 10) {
        const count = this.errorCounts.get(errorType) || 0;
        return count >= threshold;
    }

    /**
     * Create a standardized error object
     * @param {string} message - Error message
     * @param {string} code - Error code
     * @param {Object} details - Additional error details
     * @returns {Error} Standardized error object
     */
    createError(message, code = 'UNKNOWN_ERROR', details = {}) {
        const error = new Error(message);
        error.code = code;
        error.details = details;
        error.timestamp = new Date().toISOString();
        return error;
    }

    /**
     * Wrap async function with error handling and retry logic
     * @param {Function} fn - Function to wrap
     * @param {string} context - Context for error handling
     * @param {Object} options - Options for retry behavior
     * @returns {Function} Wrapped function
     */
    withRetry(fn, context, options = {}) {
        return async (...args) => {
            let lastError;
            let attempt = 1;
            const maxAttempts = options.maxAttempts || 3;

            while (attempt <= maxAttempts) {
                try {
                    return await fn(...args);
                } catch (error) {
                    lastError = error;
                    
                    const result = await this.handleError(error, context, attempt, options.metadata);
                    
                    if (!result.shouldRetry || attempt >= maxAttempts) {
                        throw error;
                    }

                    // Wait before retry
                    if (result.delay > 0) {
                        await new Promise(resolve => setTimeout(resolve, result.delay));
                    }

                    attempt++;
                }
            }

            throw lastError;
        };
    }

    /**
     * Log recovery success
     * @param {string} context - Context where recovery occurred
     * @param {number} attempt - Successful attempt number
     * @param {Object} metadata - Additional metadata
     */
    logRecovery(context, attempt, metadata = {}) {
        console.log(`✅ ErrorHandler: Recovery successful in ${context} after ${attempt} attempts`, {
            context,
            attempt,
            timestamp: new Date().toISOString(),
            metadata
        });
    }
}

module.exports = new ErrorHandlerService();
