const EventEmitter = require('events');

/**
 * CircuitBreaker - Advanced circuit breaker with multiple failure detection strategies
 * Protects system from cascading failures and provides graceful degradation
 */
class CircuitBreaker extends EventEmitter {
    constructor(options = {}) {
        super();
        
        this.config = {
            // Failure thresholds
            failureThreshold: options.failureThreshold || 5, // failures before opening
            successThreshold: options.successThreshold || 3, // successes to close from half-open
            timeout: options.timeout || 60000, // timeout before trying half-open (1 minute)
            
            // Monitoring windows
            monitoringWindow: options.monitoringWindow || 60000, // 1 minute window
            volumeThreshold: options.volumeThreshold || 10, // minimum requests before considering failure rate
            
            // Failure rate thresholds
            errorRateThreshold: options.errorRateThreshold || 0.5, // 50% error rate
            slowCallThreshold: options.slowCallThreshold || 120000, // 2 minutes for CNPJ operations
            slowCallRateThreshold: options.slowCallRateThreshold || 0.3, // 30% slow calls
            
            // Advanced features
            exponentialBackoff: options.exponentialBackoff !== false,
            maxBackoffTime: options.maxBackoffTime || 300000, // 5 minutes max
            jitterEnabled: options.jitterEnabled !== false,
            
            // Health check
            healthCheckInterval: options.healthCheckInterval || 30000, // 30 seconds
            healthCheckTimeout: options.healthCheckTimeout || 5000, // 5 seconds
            
            ...options
        };

        // Circuit states
        this.states = {
            CLOSED: 'CLOSED',     // Normal operation
            OPEN: 'OPEN',         // Failing fast
            HALF_OPEN: 'HALF_OPEN' // Testing if service recovered
        };

        // Current state
        this.state = this.states.CLOSED;
        this.failureCount = 0;
        this.successCount = 0;
        this.lastFailureTime = null;
        this.nextAttemptTime = null;
        this.backoffMultiplier = 1;

        // Request tracking
        this.requests = []; // { timestamp, duration, success, error }
        this.stats = {
            totalRequests: 0,
            successfulRequests: 0,
            failedRequests: 0,
            rejectedRequests: 0,
            averageResponseTime: 0,
            errorRate: 0,
            slowCallRate: 0,
            circuitOpenTime: 0,
            lastStateChange: Date.now()
        };

        // Health check
        this.healthCheckFunction = null;
        this.healthCheckTimer = null;
        
        this.debugLog = process.env.NODE_ENV !== 'production' ? console.log : () => {};
        
        // Start monitoring
        this.startMonitoring();
    }

    /**
     * Execute function with circuit breaker protection
     * @param {Function} fn - Function to execute
     * @param {Object} options - Execution options
     * @returns {Promise} Result or rejection
     */
    async execute(fn, options = {}) {
        const startTime = Date.now();
        
        // Check if circuit is open
        if (this.state === this.states.OPEN) {
            if (this.canAttemptReset()) {
                this.setState(this.states.HALF_OPEN);
                this.debugLog('CircuitBreaker: Transitioning to HALF_OPEN state');
            } else {
                this.stats.rejectedRequests++;
                const error = new Error('Circuit breaker is OPEN');
                error.code = 'CIRCUIT_BREAKER_OPEN';
                error.nextAttemptTime = this.nextAttemptTime;
                throw error;
            }
        }

        // Execute the function
        try {
            const result = await Promise.race([
                fn(),
                this.createTimeoutPromise(options.timeout || this.config.slowCallThreshold)
            ]);
            
            const duration = Date.now() - startTime;
            this.recordSuccess(duration);
            
            return result;
            
        } catch (error) {
            const duration = Date.now() - startTime;
            this.recordFailure(duration, error);
            throw error;
        }
    }

    /**
     * Create timeout promise
     */
    createTimeoutPromise(timeout) {
        return new Promise((_, reject) => {
            setTimeout(() => {
                reject(new Error(`Operation timed out after ${timeout}ms`));
            }, timeout);
        });
    }

    /**
     * Record successful execution
     */
    recordSuccess(duration) {
        this.requests.push({
            timestamp: Date.now(),
            duration,
            success: true,
            error: null
        });

        this.stats.totalRequests++;
        this.stats.successfulRequests++;
        this.updateAverageResponseTime(duration);

        if (this.state === this.states.HALF_OPEN) {
            this.successCount++;
            if (this.successCount >= this.config.successThreshold) {
                this.setState(this.states.CLOSED);
                this.debugLog('CircuitBreaker: Transitioning to CLOSED state after successful recovery');
            }
        } else if (this.state === this.states.CLOSED) {
            // Reset failure count on success
            this.failureCount = 0;
        }

        this.cleanOldRequests();
    }

    /**
     * Record failed execution
     */
    recordFailure(duration, error) {
        this.requests.push({
            timestamp: Date.now(),
            duration,
            success: false,
            error: error.message
        });

        this.stats.totalRequests++;
        this.stats.failedRequests++;
        this.updateAverageResponseTime(duration);

        this.failureCount++;
        this.lastFailureTime = Date.now();

        // Check if we should open the circuit
        if (this.shouldOpenCircuit()) {
            this.setState(this.states.OPEN);
            this.calculateNextAttemptTime();
            this.debugLog('CircuitBreaker: Opening circuit due to failures');
        }

        this.cleanOldRequests();
    }

    /**
     * Check if circuit should be opened
     */
    shouldOpenCircuit() {
        // Not enough requests to make a decision
        if (this.getRecentRequestCount() < this.config.volumeThreshold) {
            return false;
        }

        // Check failure count threshold
        if (this.failureCount >= this.config.failureThreshold) {
            return true;
        }

        // Check error rate
        const errorRate = this.calculateErrorRate();
        if (errorRate >= this.config.errorRateThreshold) {
            return true;
        }

        // Check slow call rate
        const slowCallRate = this.calculateSlowCallRate();
        if (slowCallRate >= this.config.slowCallRateThreshold) {
            return true;
        }

        return false;
    }

    /**
     * Check if we can attempt to reset the circuit
     */
    canAttemptReset() {
        if (!this.nextAttemptTime) {
            return true;
        }
        return Date.now() >= this.nextAttemptTime;
    }

    /**
     * Calculate next attempt time with exponential backoff
     */
    calculateNextAttemptTime() {
        let backoffTime = this.config.timeout;
        
        if (this.config.exponentialBackoff) {
            backoffTime = Math.min(
                this.config.timeout * Math.pow(2, this.backoffMultiplier - 1),
                this.config.maxBackoffTime
            );
            this.backoffMultiplier++;
        }

        // Add jitter to prevent thundering herd
        if (this.config.jitterEnabled) {
            const jitter = Math.random() * 0.1 * backoffTime; // 10% jitter
            backoffTime += jitter;
        }

        this.nextAttemptTime = Date.now() + backoffTime;
        this.debugLog(`CircuitBreaker: Next attempt in ${Math.round(backoffTime / 1000)}s`);
    }

    /**
     * Set circuit breaker state
     */
    setState(newState) {
        const oldState = this.state;
        this.state = newState;
        this.stats.lastStateChange = Date.now();

        if (newState === this.states.CLOSED) {
            this.failureCount = 0;
            this.successCount = 0;
            this.backoffMultiplier = 1;
            this.nextAttemptTime = null;
        } else if (newState === this.states.HALF_OPEN) {
            this.successCount = 0;
        } else if (newState === this.states.OPEN) {
            this.stats.circuitOpenTime = Date.now();
        }

        this.emit('stateChange', { oldState, newState, timestamp: Date.now() });
        console.log(`CircuitBreaker: State changed from ${oldState} to ${newState}`);
    }

    /**
     * Calculate error rate for recent requests
     */
    calculateErrorRate() {
        const recentRequests = this.getRecentRequests();
        if (recentRequests.length === 0) return 0;

        const failedRequests = recentRequests.filter(req => !req.success).length;
        return failedRequests / recentRequests.length;
    }

    /**
     * Calculate slow call rate for recent requests
     */
    calculateSlowCallRate() {
        const recentRequests = this.getRecentRequests();
        if (recentRequests.length === 0) return 0;

        const slowRequests = recentRequests.filter(
            req => req.duration > this.config.slowCallThreshold
        ).length;
        return slowRequests / recentRequests.length;
    }

    /**
     * Get recent requests within monitoring window
     */
    getRecentRequests() {
        const cutoff = Date.now() - this.config.monitoringWindow;
        return this.requests.filter(req => req.timestamp > cutoff);
    }

    /**
     * Get count of recent requests
     */
    getRecentRequestCount() {
        return this.getRecentRequests().length;
    }

    /**
     * Update average response time
     */
    updateAverageResponseTime(duration) {
        this.stats.averageResponseTime = this.stats.averageResponseTime === 0
            ? duration
            : (this.stats.averageResponseTime * 0.9 + duration * 0.1);
    }

    /**
     * Clean old requests outside monitoring window
     */
    cleanOldRequests() {
        const cutoff = Date.now() - this.config.monitoringWindow;
        this.requests = this.requests.filter(req => req.timestamp > cutoff);
    }

    /**
     * Start monitoring and health checks
     */
    startMonitoring() {
        // Update stats periodically
        setInterval(() => {
            this.updateStats();
        }, 5000);

        // Start health checks if configured
        if (this.healthCheckFunction) {
            this.startHealthChecks();
        }
    }

    /**
     * Update statistics
     */
    updateStats() {
        this.stats.errorRate = this.calculateErrorRate();
        this.stats.slowCallRate = this.calculateSlowCallRate();
        this.cleanOldRequests();
    }

    /**
     * Set health check function
     */
    setHealthCheck(healthCheckFn) {
        this.healthCheckFunction = healthCheckFn;
        if (this.healthCheckTimer) {
            clearInterval(this.healthCheckTimer);
        }
        this.startHealthChecks();
    }

    /**
     * Start health check monitoring
     */
    startHealthChecks() {
        if (!this.healthCheckFunction) return;

        this.healthCheckTimer = setInterval(async () => {
            if (this.state === this.states.OPEN) {
                try {
                    const healthCheckPromise = this.healthCheckFunction();
                    const timeoutPromise = new Promise((_, reject) => {
                        setTimeout(() => reject(new Error('Health check timeout')), 
                                 this.config.healthCheckTimeout);
                    });

                    await Promise.race([healthCheckPromise, timeoutPromise]);
                    
                    // Health check passed, try to recover
                    this.debugLog('CircuitBreaker: Health check passed, attempting recovery');
                    this.setState(this.states.HALF_OPEN);
                    
                } catch (error) {
                    this.debugLog('CircuitBreaker: Health check failed:', error.message);
                    // Extend the wait time
                    this.calculateNextAttemptTime();
                }
            }
        }, this.config.healthCheckInterval);
    }

    /**
     * Force circuit state (for testing/emergency)
     */
    forceState(state) {
        if (Object.values(this.states).includes(state)) {
            this.setState(state);
            console.log(`CircuitBreaker: Forced state to ${state}`);
        } else {
            throw new Error(`Invalid state: ${state}`);
        }
    }

    /**
     * Reset circuit breaker to initial state
     */
    reset() {
        this.setState(this.states.CLOSED);
        this.requests = [];
        this.stats = {
            totalRequests: 0,
            successfulRequests: 0,
            failedRequests: 0,
            rejectedRequests: 0,
            averageResponseTime: 0,
            errorRate: 0,
            slowCallRate: 0,
            circuitOpenTime: 0,
            lastStateChange: Date.now()
        };
        console.log('CircuitBreaker: Reset to initial state');
    }

    /**
     * Get comprehensive statistics
     */
    getStats() {
        return {
            state: this.state,
            failureCount: this.failureCount,
            successCount: this.successCount,
            nextAttemptTime: this.nextAttemptTime,
            recentRequestCount: this.getRecentRequestCount(),
            ...this.stats,
            config: this.config
        };
    }

    /**
     * Shutdown circuit breaker
     */
    shutdown() {
        if (this.healthCheckTimer) {
            clearInterval(this.healthCheckTimer);
        }
        console.log('CircuitBreaker: Shutdown completed');
    }
}

module.exports = CircuitBreaker;
