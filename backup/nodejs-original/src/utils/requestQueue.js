const EventEmitter = require('events');

/**
 * RequestQueue - Advanced queue system with priority, throttling, and backpressure
 * Handles burst traffic and prevents system overload
 */
class RequestQueue extends EventEmitter {
    constructor(options = {}) {
        super();
        
        this.config = {
            maxQueueSize: options.maxQueueSize || 1000,
            maxConcurrent: options.maxConcurrent || 20,
            throttleRate: options.throttleRate || 10, // requests per second
            priorityLevels: options.priorityLevels || 5,
            queueTimeout: options.queueTimeout || 300000, // 5 minutes
            backpressureThreshold: options.backpressureThreshold || 0.8, // 80% of maxQueueSize
            adaptiveThrottling: options.adaptiveThrottling !== false,
            ...options
        };

        // Queue state
        this.queues = new Map(); // priority -> array of requests
        this.processing = new Set(); // currently processing requests
        this.stats = {
            totalRequests: 0,
            processedRequests: 0,
            rejectedRequests: 0,
            timeoutRequests: 0,
            averageWaitTime: 0,
            averageProcessTime: 0,
            peakQueueSize: 0,
            currentQueueSize: 0,
            throughput: 0, // requests per second
            backpressureEvents: 0
        };

        // Throttling state
        this.lastProcessTime = 0;
        this.throttleInterval = 1000 / this.config.throttleRate;
        this.adaptiveThrottleRate = this.config.throttleRate;
        
        // Initialize priority queues
        for (let i = 0; i < this.config.priorityLevels; i++) {
            this.queues.set(i, []);
        }

        // Start processing
        this.isProcessing = false;
        this.startProcessing();
        
        // Throughput calculation
        this.throughputWindow = [];
        this.throughputTimer = setInterval(() => {
            this.calculateThroughput();
        }, 1000);

        this.debugLog = process.env.NODE_ENV !== 'production' ? console.log : () => {};
    }

    /**
     * Add request to queue with priority and options
     * @param {Function} processor - Function to process the request
     * @param {Object} options - Request options
     * @returns {Promise} Promise that resolves when request is processed
     */
    async enqueue(processor, options = {}) {
        const request = {
            id: `req-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`,
            processor,
            priority: Math.max(0, Math.min(options.priority || 2, this.config.priorityLevels - 1)),
            enqueuedAt: Date.now(),
            timeout: options.timeout || this.config.queueTimeout,
            metadata: options.metadata || {},
            resolve: null,
            reject: null,
            timeoutHandle: null
        };

        // Check backpressure
        if (this.isBackpressureActive()) {
            this.stats.backpressureEvents++;
            this.emit('backpressure', { queueSize: this.getTotalQueueSize() });
            
            if (options.dropOnBackpressure) {
                this.stats.rejectedRequests++;
                throw new Error('Request dropped due to backpressure');
            }
        }

        // Check queue size limit
        if (this.getTotalQueueSize() >= this.config.maxQueueSize) {
            this.stats.rejectedRequests++;
            throw new Error('Queue is full');
        }

        return new Promise((resolve, reject) => {
            request.resolve = resolve;
            request.reject = reject;

            // Set timeout
            request.timeoutHandle = setTimeout(() => {
                this.removeRequest(request);
                this.stats.timeoutRequests++;
                reject(new Error('Request timeout in queue'));
            }, request.timeout);

            // Add to appropriate priority queue
            const priorityQueue = this.queues.get(request.priority);
            priorityQueue.push(request);

            this.stats.totalRequests++;
            this.stats.currentQueueSize = this.getTotalQueueSize();
            this.stats.peakQueueSize = Math.max(this.stats.peakQueueSize, this.stats.currentQueueSize);

            this.debugLog(`RequestQueue: Enqueued request ${request.id} with priority ${request.priority}`);
            this.emit('enqueued', { requestId: request.id, priority: request.priority, queueSize: this.stats.currentQueueSize });

            // Start processing if not already running
            if (!this.isProcessing) {
                this.startProcessing();
            }
        });
    }

    /**
     * Start processing requests from queues
     */
    startProcessing() {
        if (this.isProcessing) return;
        
        this.isProcessing = true;
        this.processNext();
    }

    /**
     * Process next request from highest priority queue
     */
    async processNext() {
        if (!this.isProcessing) return;

        // Check if we can process more requests
        if (this.processing.size >= this.config.maxConcurrent) {
            // Wait a bit and try again
            setTimeout(() => this.processNext(), 10);
            return;
        }

        // Find next request to process (highest priority first)
        const request = this.getNextRequest();
        if (!request) {
            // No requests to process, check again later
            setTimeout(() => this.processNext(), 50);
            return;
        }

        // Apply throttling
        await this.applyThrottling();

        // Process the request
        this.processRequest(request);

        // Continue processing
        setImmediate(() => this.processNext());
    }

    /**
     * Get next request from highest priority queue
     */
    getNextRequest() {
        // Check queues from highest to lowest priority
        for (let priority = this.config.priorityLevels - 1; priority >= 0; priority--) {
            const queue = this.queues.get(priority);
            if (queue.length > 0) {
                return queue.shift();
            }
        }
        return null;
    }

    /**
     * Process individual request
     */
    async processRequest(request) {
        const startTime = Date.now();
        this.processing.add(request.id);
        
        // Clear timeout
        if (request.timeoutHandle) {
            clearTimeout(request.timeoutHandle);
        }

        try {
            this.debugLog(`RequestQueue: Processing request ${request.id}`);
            
            const result = await request.processor();
            
            const processTime = Date.now() - startTime;
            const waitTime = startTime - request.enqueuedAt;
            
            this.updateStats(waitTime, processTime);
            
            request.resolve(result);
            this.emit('processed', { 
                requestId: request.id, 
                waitTime, 
                processTime,
                priority: request.priority 
            });
            
        } catch (error) {
            const processTime = Date.now() - startTime;
            const waitTime = startTime - request.enqueuedAt;
            
            this.updateStats(waitTime, processTime);
            
            request.reject(error);
            this.emit('error', { 
                requestId: request.id, 
                error: error.message,
                waitTime,
                processTime,
                priority: request.priority 
            });
            
        } finally {
            this.processing.delete(request.id);
            this.stats.processedRequests++;
            this.stats.currentQueueSize = this.getTotalQueueSize();
            
            // Record for throughput calculation
            this.throughputWindow.push(Date.now());
        }
    }

    /**
     * Apply adaptive throttling
     */
    async applyThrottling() {
        const now = Date.now();
        const timeSinceLastProcess = now - this.lastProcessTime;
        
        // Calculate current throttle interval
        let currentInterval = this.throttleInterval;
        
        if (this.config.adaptiveThrottling) {
            // Adjust throttling based on queue size and system load
            const queueLoadFactor = this.getTotalQueueSize() / this.config.maxQueueSize;
            const concurrencyFactor = this.processing.size / this.config.maxConcurrent;
            
            // Reduce throttling when system is under load
            const adaptiveFactor = Math.max(0.1, 1 - (queueLoadFactor * 0.5 + concurrencyFactor * 0.3));
            currentInterval = this.throttleInterval * adaptiveFactor;
            
            this.adaptiveThrottleRate = 1000 / currentInterval;
        }
        
        if (timeSinceLastProcess < currentInterval) {
            const waitTime = currentInterval - timeSinceLastProcess;
            await new Promise(resolve => setTimeout(resolve, waitTime));
        }
        
        this.lastProcessTime = Date.now();
    }

    /**
     * Remove request from queue
     */
    removeRequest(request) {
        for (const [priority, queue] of this.queues.entries()) {
            const index = queue.findIndex(r => r.id === request.id);
            if (index !== -1) {
                queue.splice(index, 1);
                this.stats.currentQueueSize = this.getTotalQueueSize();
                return true;
            }
        }
        return false;
    }

    /**
     * Check if backpressure is active
     */
    isBackpressureActive() {
        const queueSize = this.getTotalQueueSize();
        const threshold = this.config.maxQueueSize * this.config.backpressureThreshold;
        return queueSize >= threshold;
    }

    /**
     * Get total queue size across all priorities
     */
    getTotalQueueSize() {
        let total = 0;
        for (const queue of this.queues.values()) {
            total += queue.length;
        }
        return total;
    }

    /**
     * Update statistics
     */
    updateStats(waitTime, processTime) {
        // Exponential moving average
        this.stats.averageWaitTime = this.stats.averageWaitTime === 0 
            ? waitTime 
            : (this.stats.averageWaitTime * 0.9 + waitTime * 0.1);
            
        this.stats.averageProcessTime = this.stats.averageProcessTime === 0 
            ? processTime 
            : (this.stats.averageProcessTime * 0.9 + processTime * 0.1);
    }

    /**
     * Calculate throughput (requests per second)
     */
    calculateThroughput() {
        const now = Date.now();
        const oneSecondAgo = now - 1000;
        
        // Remove old entries
        this.throughputWindow = this.throughputWindow.filter(time => time > oneSecondAgo);
        
        this.stats.throughput = this.throughputWindow.length;
    }

    /**
     * Get comprehensive queue statistics
     */
    getStats() {
        const queueSizes = {};
        for (const [priority, queue] of this.queues.entries()) {
            queueSizes[`priority_${priority}`] = queue.length;
        }

        return {
            ...this.stats,
            processing: this.processing.size,
            queueSizes,
            adaptiveThrottleRate: this.adaptiveThrottleRate,
            backpressureActive: this.isBackpressureActive(),
            config: this.config
        };
    }

    /**
     * Clear all queues (emergency use only)
     */
    clearAll() {
        let clearedCount = 0;
        
        for (const queue of this.queues.values()) {
            for (const request of queue) {
                if (request.timeoutHandle) {
                    clearTimeout(request.timeoutHandle);
                }
                request.reject(new Error('Queue cleared'));
                clearedCount++;
            }
            queue.length = 0;
        }
        
        this.stats.currentQueueSize = 0;
        this.stats.rejectedRequests += clearedCount;
        
        console.log(`RequestQueue: Cleared ${clearedCount} requests`);
        this.emit('cleared', { clearedCount });
        
        return clearedCount;
    }

    /**
     * Graceful shutdown
     */
    async shutdown() {
        console.log('RequestQueue: Starting graceful shutdown...');
        this.isProcessing = false;
        
        // Clear throughput timer
        if (this.throughputTimer) {
            clearInterval(this.throughputTimer);
        }
        
        // Wait for current processing to complete
        while (this.processing.size > 0) {
            await new Promise(resolve => setTimeout(resolve, 100));
        }
        
        // Reject all remaining queued requests
        const rejectedCount = this.clearAll();
        
        console.log(`RequestQueue: Shutdown completed, rejected ${rejectedCount} pending requests`);
        this.emit('shutdown', { rejectedCount });
    }
}

module.exports = RequestQueue;
