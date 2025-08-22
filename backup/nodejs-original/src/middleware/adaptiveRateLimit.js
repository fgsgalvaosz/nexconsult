/**
 * AdaptiveRateLimit - Intelligent rate limiting based on system resources
 * Dynamically adjusts limits based on server load, queue size, and performance metrics
 */
class AdaptiveRateLimit {
    constructor(options = {}) {
        this.config = {
            // Base rate limits
            baseWindowMs: options.baseWindowMs || 60000, // 1 minute
            baseMaxRequests: options.baseMaxRequests || 100,
            
            // Adaptive scaling factors
            cpuThreshold: options.cpuThreshold || 0.8, // 80% CPU usage
            memoryThreshold: options.memoryThreshold || 0.8, // 80% memory usage
            queueThreshold: options.queueThreshold || 0.7, // 70% queue capacity
            
            // Scaling factors
            highLoadFactor: options.highLoadFactor || 0.3, // Reduce to 30% under high load
            mediumLoadFactor: options.mediumLoadFactor || 0.6, // Reduce to 60% under medium load
            lowLoadFactor: options.lowLoadFactor || 1.2, // Increase to 120% under low load
            
            // VIP and priority handling
            vipMultiplier: options.vipMultiplier || 3, // VIP users get 3x limit
            priorityBypass: options.priorityBypass !== false,
            
            // Monitoring
            metricsWindow: options.metricsWindow || 5000, // 5 seconds
            adaptationInterval: options.adaptationInterval || 10000, // 10 seconds
            
            ...options
        };

        // State tracking
        this.clients = new Map(); // IP -> { requests: [], lastReset: timestamp, tier: 'normal'|'vip' }
        this.systemMetrics = {
            cpuUsage: 0,
            memoryUsage: 0,
            queueLoad: 0,
            responseTime: 0,
            errorRate: 0
        };
        
        this.currentLimits = {
            windowMs: this.config.baseWindowMs,
            maxRequests: this.config.baseMaxRequests,
            lastAdaptation: Date.now()
        };

        // Performance tracking
        this.performanceWindow = [];
        this.errorWindow = [];
        
        // Start monitoring
        this.startSystemMonitoring();
        this.startAdaptation();
        
        this.debugLog = process.env.NODE_ENV !== 'production' ? console.log : () => {};
    }

    /**
     * Express middleware function
     */
    middleware() {
        return async (req, res, next) => {
            const startTime = Date.now();
            const clientId = this.getClientId(req);
            const clientTier = this.getClientTier(req);
            
            try {
                // Check if request should be allowed
                const allowed = await this.checkRateLimit(clientId, clientTier, req);
                
                if (!allowed.success) {
                    // Rate limit exceeded
                    this.setRateLimitHeaders(res, allowed);
                    return res.status(429).json({
                        error: 'Rate limit exceeded',
                        message: allowed.message,
                        retryAfter: allowed.retryAfter,
                        limit: allowed.limit,
                        remaining: allowed.remaining,
                        resetTime: allowed.resetTime
                    });
                }

                // Set rate limit headers for successful requests
                this.setRateLimitHeaders(res, allowed);
                
                // Track response time and errors
                const originalSend = res.send;
                res.send = (body) => {
                    const responseTime = Date.now() - startTime;
                    this.recordPerformance(responseTime, res.statusCode >= 400);
                    return originalSend.call(res, body);
                };
                
                next();
                
            } catch (error) {
                console.error('AdaptiveRateLimit: Error in middleware:', error.message);
                next(); // Allow request to proceed on rate limiter error
            }
        };
    }

    /**
     * Check if request is within rate limit
     */
    async checkRateLimit(clientId, clientTier, req) {
        const now = Date.now();
        const client = this.getOrCreateClient(clientId, clientTier);
        
        // Calculate effective limits for this client
        const effectiveLimits = this.calculateEffectiveLimits(clientTier, req);
        
        // Clean old requests outside window
        client.requests = client.requests.filter(
            timestamp => now - timestamp < effectiveLimits.windowMs
        );
        
        // Check if limit exceeded
        if (client.requests.length >= effectiveLimits.maxRequests) {
            const oldestRequest = Math.min(...client.requests);
            const retryAfter = Math.ceil((oldestRequest + effectiveLimits.windowMs - now) / 1000);
            
            return {
                success: false,
                message: `Rate limit exceeded. Try again in ${retryAfter} seconds.`,
                limit: effectiveLimits.maxRequests,
                remaining: 0,
                retryAfter,
                resetTime: new Date(oldestRequest + effectiveLimits.windowMs).toISOString()
            };
        }
        
        // Allow request
        client.requests.push(now);
        client.lastSeen = now;
        
        return {
            success: true,
            limit: effectiveLimits.maxRequests,
            remaining: effectiveLimits.maxRequests - client.requests.length,
            resetTime: new Date(now + effectiveLimits.windowMs).toISOString()
        };
    }

    /**
     * Calculate effective limits based on system load and client tier
     */
    calculateEffectiveLimits(clientTier, req) {
        let baseLimits = {
            windowMs: this.currentLimits.windowMs,
            maxRequests: this.currentLimits.maxRequests
        };

        // Apply tier multipliers
        if (clientTier === 'vip') {
            baseLimits.maxRequests *= this.config.vipMultiplier;
        }

        // Apply priority bypass for critical endpoints
        if (this.config.priorityBypass && this.isCriticalEndpoint(req)) {
            baseLimits.maxRequests *= 2;
        }

        return baseLimits;
    }

    /**
     * Get or create client tracking record
     */
    getOrCreateClient(clientId, tier) {
        if (!this.clients.has(clientId)) {
            this.clients.set(clientId, {
                requests: [],
                tier,
                lastSeen: Date.now(),
                totalRequests: 0
            });
        }
        
        const client = this.clients.get(clientId);
        client.totalRequests++;
        return client;
    }

    /**
     * Get client identifier (IP + User-Agent hash for better uniqueness)
     */
    getClientId(req) {
        const ip = req.ip || req.connection.remoteAddress || 'unknown';
        const userAgent = req.get('User-Agent') || '';
        
        // Simple hash of user agent for better client identification
        const uaHash = userAgent.split('').reduce((hash, char) => {
            return ((hash << 5) - hash) + char.charCodeAt(0);
        }, 0);
        
        return `${ip}:${Math.abs(uaHash) % 10000}`;
    }

    /**
     * Determine client tier (normal, vip, etc.)
     */
    getClientTier(req) {
        // Check for VIP indicators
        const apiKey = req.headers['x-api-key'] || req.body?.apiKey;
        const authHeader = req.headers.authorization;
        
        // Simple VIP detection (can be enhanced with database lookup)
        if (apiKey && apiKey.length > 20) {
            return 'vip';
        }
        
        if (authHeader && authHeader.includes('Bearer')) {
            return 'vip';
        }
        
        return 'normal';
    }

    /**
     * Check if endpoint is critical (health checks, status, etc.)
     */
    isCriticalEndpoint(req) {
        const criticalPaths = ['/health', '/status', '/api/cnpj/status'];
        return criticalPaths.some(path => req.path.includes(path));
    }

    /**
     * Set rate limit headers
     */
    setRateLimitHeaders(res, result) {
        res.set({
            'X-RateLimit-Limit': result.limit,
            'X-RateLimit-Remaining': result.remaining || 0,
            'X-RateLimit-Reset': result.resetTime,
            'X-RateLimit-Policy': 'adaptive'
        });
        
        if (result.retryAfter) {
            res.set('Retry-After', result.retryAfter);
        }
    }

    /**
     * Record performance metrics
     */
    recordPerformance(responseTime, isError) {
        const now = Date.now();
        
        this.performanceWindow.push({ time: now, responseTime });
        this.errorWindow.push({ time: now, isError });
        
        // Keep only recent data
        const cutoff = now - this.config.metricsWindow;
        this.performanceWindow = this.performanceWindow.filter(entry => entry.time > cutoff);
        this.errorWindow = this.errorWindow.filter(entry => entry.time > cutoff);
    }

    /**
     * Start system monitoring
     */
    startSystemMonitoring() {
        setInterval(() => {
            this.updateSystemMetrics();
        }, this.config.metricsWindow);
    }

    /**
     * Update system metrics
     */
    updateSystemMetrics() {
        // CPU and Memory usage
        const memUsage = process.memoryUsage();
        this.systemMetrics.memoryUsage = memUsage.heapUsed / memUsage.heapTotal;
        
        // Response time (average of recent requests)
        if (this.performanceWindow.length > 0) {
            const avgResponseTime = this.performanceWindow.reduce((sum, entry) => 
                sum + entry.responseTime, 0) / this.performanceWindow.length;
            this.systemMetrics.responseTime = avgResponseTime;
        }
        
        // Error rate
        if (this.errorWindow.length > 0) {
            const errorCount = this.errorWindow.filter(entry => entry.isError).length;
            this.systemMetrics.errorRate = errorCount / this.errorWindow.length;
        }
        
        // Queue load (if available from external source)
        // This would be injected by the main application
        this.systemMetrics.queueLoad = this.externalQueueLoad || 0;
    }

    /**
     * Start adaptive rate limit adjustment
     */
    startAdaptation() {
        setInterval(() => {
            this.adaptRateLimits();
        }, this.config.adaptationInterval);
    }

    /**
     * Adapt rate limits based on system metrics
     */
    adaptRateLimits() {
        const metrics = this.systemMetrics;
        let scaleFactor = 1.0;
        
        // Determine system load level
        const highLoad = metrics.memoryUsage > this.config.memoryThreshold ||
                        metrics.queueLoad > this.config.queueThreshold ||
                        metrics.errorRate > 0.1 ||
                        metrics.responseTime > 5000;
                        
        const mediumLoad = metrics.memoryUsage > this.config.memoryThreshold * 0.6 ||
                          metrics.queueLoad > this.config.queueThreshold * 0.5 ||
                          metrics.errorRate > 0.05 ||
                          metrics.responseTime > 2000;
        
        if (highLoad) {
            scaleFactor = this.config.highLoadFactor;
            this.debugLog('AdaptiveRateLimit: High load detected, reducing limits');
        } else if (mediumLoad) {
            scaleFactor = this.config.mediumLoadFactor;
            this.debugLog('AdaptiveRateLimit: Medium load detected, moderately reducing limits');
        } else {
            scaleFactor = this.config.lowLoadFactor;
            this.debugLog('AdaptiveRateLimit: Low load detected, increasing limits');
        }
        
        // Apply scaling
        const newMaxRequests = Math.max(10, Math.floor(this.config.baseMaxRequests * scaleFactor));
        
        if (newMaxRequests !== this.currentLimits.maxRequests) {
            this.currentLimits.maxRequests = newMaxRequests;
            this.currentLimits.lastAdaptation = Date.now();
            
            console.log(`AdaptiveRateLimit: Adjusted limits to ${newMaxRequests} requests per window (scale: ${scaleFactor.toFixed(2)})`);
        }
    }

    /**
     * Set external queue load (to be called by main application)
     */
    setQueueLoad(queueLoad) {
        this.externalQueueLoad = queueLoad;
    }

    /**
     * Get comprehensive statistics
     */
    getStats() {
        const now = Date.now();
        
        // Clean up old clients
        for (const [clientId, client] of this.clients.entries()) {
            if (now - client.lastSeen > this.config.baseWindowMs * 2) {
                this.clients.delete(clientId);
            }
        }
        
        return {
            currentLimits: this.currentLimits,
            systemMetrics: this.systemMetrics,
            activeClients: this.clients.size,
            totalClients: this.clients.size,
            config: this.config,
            performanceWindow: this.performanceWindow.length,
            errorWindow: this.errorWindow.length
        };
    }

    /**
     * Reset all rate limits (emergency use)
     */
    reset() {
        this.clients.clear();
        this.performanceWindow = [];
        this.errorWindow = [];
        this.currentLimits.maxRequests = this.config.baseMaxRequests;
        console.log('AdaptiveRateLimit: All limits reset');
    }
}

module.exports = AdaptiveRateLimit;
