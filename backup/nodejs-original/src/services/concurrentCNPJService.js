const AdvancedBrowserPool = require('../utils/advancedBrowserPool');
const RequestQueue = require('../utils/requestQueue');
const AdaptiveRateLimit = require('../middleware/adaptiveRateLimit');
const CircuitBreaker = require('../utils/circuitBreaker');
const RealTimeMonitor = require('../utils/realTimeMonitor');
const CNPJService = require('./cnpjService');

/**
 * ConcurrentCNPJService - High-performance CNPJ service for handling concurrent requests
 * Integrates all optimization components for maximum throughput and reliability
 */
class ConcurrentCNPJService {
    constructor(options = {}) {
        this.config = {
            // Browser pool configuration
            minBrowsers: options.minBrowsers || 5,
            maxBrowsers: options.maxBrowsers || 20,
            maxPagesPerBrowser: options.maxPagesPerBrowser || 3,
            
            // Queue configuration
            maxQueueSize: options.maxQueueSize || 500,
            maxConcurrent: options.maxConcurrent || 30,
            throttleRate: options.throttleRate || 15, // requests per second
            
            // Circuit breaker configuration
            failureThreshold: options.failureThreshold || 10,
            timeout: options.timeout || 300000, // 5 minutes for CNPJ operations
            
            // Rate limiting
            baseMaxRequests: options.baseMaxRequests || 200,
            baseWindowMs: options.baseWindowMs || 60000, // 1 minute
            
            ...options
        };

        // Initialize components
        this.browserPool = new AdvancedBrowserPool({
            minBrowsers: this.config.minBrowsers,
            maxBrowsers: this.config.maxBrowsers,
            maxPagesPerBrowser: this.config.maxPagesPerBrowser,
            warmupBrowsers: Math.min(this.config.minBrowsers, 5)
        });

        this.requestQueue = new RequestQueue({
            maxQueueSize: this.config.maxQueueSize,
            maxConcurrent: this.config.maxConcurrent,
            throttleRate: this.config.throttleRate,
            adaptiveThrottling: true
        });

        this.rateLimit = new AdaptiveRateLimit({
            baseMaxRequests: this.config.baseMaxRequests,
            baseWindowMs: this.config.baseWindowMs,
            adaptationInterval: 15000 // 15 seconds
        });

        this.circuitBreaker = new CircuitBreaker({
            failureThreshold: this.config.failureThreshold,
            timeout: this.config.timeout,
            exponentialBackoff: true
        });

        this.monitor = new RealTimeMonitor({
            metricsInterval: 2000, // 2 seconds
            healthCheckInterval: 10000, // 10 seconds
            alertCheckInterval: 15000 // 15 seconds
        });

        // Legacy CNPJ service for actual processing
        this.cnpjService = CNPJService;

        // Statistics
        this.stats = {
            totalRequests: 0,
            successfulRequests: 0,
            failedRequests: 0,
            queuedRequests: 0,
            rejectedRequests: 0,
            averageResponseTime: 0,
            startTime: Date.now()
        };

        // Initialize system
        this.isInitialized = false;
        this.isShuttingDown = false;
    }

    /**
     * Initialize the concurrent service
     */
    async initialize() {
        if (this.isInitialized) return;

        console.log('🚀 ConcurrentCNPJService: Initializing high-performance system...');

        try {
            // Initialize browser pool
            await this.browserPool.initialize();

            // Register data sources with monitor
            this.monitor.registerDataSource('browserPool', this.browserPool);
            this.monitor.registerDataSource('requestQueue', this.requestQueue);
            this.monitor.registerDataSource('circuitBreaker', this.circuitBreaker);
            this.monitor.registerDataSource('cnpjService', this.cnpjService);

            // Setup circuit breaker health check
            this.circuitBreaker.setHealthCheck(async () => {
                // Simple health check - try to get browser pool stats
                const stats = this.browserPool.getStats();
                if (stats.healthyBrowsers === 0) {
                    throw new Error('No healthy browsers available');
                }
            });

            // Setup event listeners
            this.setupEventListeners();

            // Update rate limiter with queue load
            setInterval(() => {
                const queueStats = this.requestQueue.getStats();
                const queueLoad = queueStats.currentQueueSize / queueStats.config.maxQueueSize;
                this.rateLimit.setQueueLoad(queueLoad);
            }, 5000);

            this.isInitialized = true;
            console.log('✅ ConcurrentCNPJService: Initialization completed');

        } catch (error) {
            console.error('❌ ConcurrentCNPJService: Initialization failed:', error.message);
            throw error;
        }
    }

    /**
     * Setup event listeners for monitoring and alerting
     */
    setupEventListeners() {
        // Browser pool events
        this.browserPool.on('browserCreated', (data) => {
            console.log(`🌐 Browser created: ${data.browserId} (Total: ${data.totalBrowsers})`);
        });

        this.browserPool.on('browserDisconnected', (data) => {
            console.warn(`⚠️ Browser disconnected: ${data.browserId}`);
        });

        // Request queue events
        this.requestQueue.on('backpressure', (data) => {
            console.warn(`🚨 Backpressure detected: Queue size ${data.queueSize}`);
        });

        // Circuit breaker events
        this.circuitBreaker.on('stateChange', (data) => {
            console.log(`🔄 Circuit breaker: ${data.oldState} → ${data.newState}`);
        });

        // Monitor events
        this.monitor.on('alert', (alert) => {
            console.warn(`🚨 ALERT [${alert.severity}]: ${alert.message}`);
        });

        this.monitor.on('alertResolved', (alert) => {
            console.log(`✅ RESOLVED: ${alert.message}`);
        });
    }

    /**
     * Process CNPJ consultation with full concurrency support
     * @param {string} cnpj - CNPJ to consult
     * @param {string} apiKey - Captcha API key
     * @param {Object} options - Request options
     * @returns {Promise<Object>} Consultation result
     */
    async consultarCNPJ(cnpj, apiKey, options = {}) {
        if (!this.isInitialized) {
            await this.initialize();
        }

        if (this.isShuttingDown) {
            throw new Error('Service is shutting down');
        }

        const startTime = Date.now();
        const requestId = `req-${Date.now()}-${Math.random().toString(36).substring(2, 11)}`;
        
        this.stats.totalRequests++;

        try {
            // Execute with circuit breaker protection
            const result = await this.circuitBreaker.execute(async () => {
                // Queue the request for processing
                return await this.requestQueue.enqueue(async () => {
                    return await this.processCNPJRequest(cnpj, apiKey, options);
                }, {
                    priority: options.priority || 2,
                    timeout: options.timeout || 300000, // 5 minutes
                    metadata: { cnpj, requestId }
                });
            });

            const responseTime = Date.now() - startTime;
            this.updateStats(responseTime, true);
            
            console.log(`✅ CNPJ consultation completed: ${cnpj} in ${responseTime}ms`);
            return result;

        } catch (error) {
            const responseTime = Date.now() - startTime;
            this.updateStats(responseTime, false);
            
            console.error(`❌ CNPJ consultation failed: ${cnpj} - ${error.message}`);
            
            // Enhance error with context
            error.requestId = requestId;
            error.cnpj = cnpj;
            error.responseTime = responseTime;
            
            throw error;
        }
    }

    /**
     * Process individual CNPJ request
     */
    async processCNPJRequest(cnpj, apiKey, options) {
        // Get browser page from pool
        const { browserId, releasePage } = await this.browserPool.getPage({
            priority: options.priority
        });

        try {
            // Use the legacy CNPJ service for actual processing
            const result = await this.cnpjService.consultarCNPJ(cnpj, apiKey);

            return {
                ...result,
                metadata: {
                    browserId,
                    processedAt: new Date().toISOString(),
                    version: '2.0-concurrent',
                    processingMode: 'high-concurrency'
                }
            };

        } finally {
            // Always release the page back to pool
            if (releasePage) {
                await releasePage();
            }
        }
    }

    /**
     * Update service statistics
     */
    updateStats(responseTime, success) {
        if (success) {
            this.stats.successfulRequests++;
        } else {
            this.stats.failedRequests++;
        }

        // Update average response time (exponential moving average)
        this.stats.averageResponseTime = this.stats.averageResponseTime === 0
            ? responseTime
            : (this.stats.averageResponseTime * 0.9 + responseTime * 0.1);
    }

    /**
     * Get rate limiting middleware
     */
    getRateLimitMiddleware() {
        return this.rateLimit.middleware();
    }

    /**
     * Get comprehensive service statistics
     */
    getStats() {
        const uptime = Date.now() - this.stats.startTime;
        const throughput = this.stats.totalRequests / (uptime / 1000); // requests per second

        return {
            service: {
                ...this.stats,
                uptime,
                throughput,
                successRate: this.stats.totalRequests > 0 
                    ? (this.stats.successfulRequests / this.stats.totalRequests * 100).toFixed(2) + '%'
                    : '0%'
            },
            browserPool: this.browserPool.getStats(),
            requestQueue: this.requestQueue.getStats(),
            circuitBreaker: this.circuitBreaker.getStats(),
            rateLimit: this.rateLimit.getStats(),
            monitor: this.monitor.getStats()
        };
    }

    /**
     * Get real-time dashboard data
     */
    getDashboardData() {
        return this.monitor.getDashboardData();
    }

    /**
     * Get health status
     */
    getHealth() {
        const stats = this.getStats();
        
        return {
            status: this.circuitBreaker.state === 'CLOSED' ? 'healthy' : 'degraded',
            uptime: stats.service.uptime,
            components: {
                browserPool: {
                    status: stats.browserPool.healthyBrowsers > 0 ? 'healthy' : 'unhealthy',
                    browsers: `${stats.browserPool.healthyBrowsers}/${stats.browserPool.totalBrowsers}`
                },
                requestQueue: {
                    status: stats.requestQueue.currentQueueSize < stats.requestQueue.config.maxQueueSize * 0.8 ? 'healthy' : 'warning',
                    queueSize: stats.requestQueue.currentQueueSize
                },
                circuitBreaker: {
                    status: stats.circuitBreaker.state,
                    errorRate: `${(stats.circuitBreaker.errorRate * 100).toFixed(1)}%`
                }
            },
            metrics: {
                totalRequests: stats.service.totalRequests,
                successRate: stats.service.successRate,
                averageResponseTime: `${Math.round(stats.service.averageResponseTime)}ms`,
                throughput: `${stats.service.throughput.toFixed(2)} req/s`
            }
        };
    }

    /**
     * Graceful shutdown
     */
    async shutdown() {
        if (this.isShuttingDown) return;
        
        console.log('ConcurrentCNPJService: Starting graceful shutdown...');
        this.isShuttingDown = true;

        try {
            // Stop accepting new requests
            await this.requestQueue.shutdown();
            
            // Close circuit breaker
            this.circuitBreaker.shutdown();
            
            // Stop monitoring
            this.monitor.shutdown();
            
            // Shutdown browser pool
            await this.browserPool.shutdown();
            
            console.log('ConcurrentCNPJService: Shutdown completed');

        } catch (error) {
            console.error('ConcurrentCNPJService: Error during shutdown:', error.message);
        }
    }
}

module.exports = ConcurrentCNPJService;
