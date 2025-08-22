const puppeteer = require('puppeteer');
const EventEmitter = require('events');

/**
 * AdvancedBrowserPool - High-performance browser pool for handling concurrent requests
 * Supports 10-20 browsers with intelligent queue management and load balancing
 */
class AdvancedBrowserPool extends EventEmitter {
    constructor(options = {}) {
        super();
        
        // Configuration with production-ready defaults
        this.config = {
            minBrowsers: options.minBrowsers || 3,
            maxBrowsers: options.maxBrowsers || 15,
            maxPagesPerBrowser: options.maxPagesPerBrowser || 3,
            browserTimeout: options.browserTimeout || 60000,
            pageTimeout: options.pageTimeout || 30000,
            idleTimeout: options.idleTimeout || 5 * 60 * 1000, // 5 minutes
            warmupBrowsers: options.warmupBrowsers || 5,
            queueTimeout: options.queueTimeout || 120000, // 2 minutes max wait
            healthCheckInterval: options.healthCheckInterval || 30000, // 30 seconds
            ...options
        };

        // Pool state
        this.browsers = new Map(); // browserId -> { browser, pages: Set, lastUsed, isHealthy }
        this.availableBrowsers = new Set(); // browserIds with available capacity
        this.requestQueue = []; // { resolve, reject, timestamp, priority }
        this.stats = {
            totalBrowsersCreated: 0,
            totalPagesCreated: 0,
            totalRequestsServed: 0,
            queuedRequests: 0,
            averageWaitTime: 0,
            peakConcurrency: 0,
            currentConcurrency: 0,
            healthyBrowsers: 0,
            unhealthyBrowsers: 0
        };

        // State management
        this.isInitialized = false;
        this.isShuttingDown = false;
        this.healthCheckTimer = null;
        
        this.debugLog = process.env.NODE_ENV !== 'production' ? console.log : () => {};
    }

    /**
     * Initialize the browser pool with warmup browsers
     */
    async initialize() {
        if (this.isInitialized) return;
        
        console.log(`🚀 AdvancedBrowserPool: Initializing with ${this.config.warmupBrowsers} warmup browsers...`);
        
        try {
            // Create initial browsers in parallel
            const warmupPromises = [];
            for (let i = 0; i < this.config.warmupBrowsers; i++) {
                warmupPromises.push(this.createBrowser());
            }
            
            await Promise.allSettled(warmupPromises);
            
            // Start health monitoring
            this.startHealthMonitoring();
            
            this.isInitialized = true;
            console.log(`✅ AdvancedBrowserPool: Initialized with ${this.browsers.size} browsers`);
            this.emit('initialized', { browserCount: this.browsers.size });
            
        } catch (error) {
            console.error('❌ AdvancedBrowserPool: Initialization failed:', error.message);
            throw error;
        }
    }

    /**
     * Get a browser page with intelligent load balancing
     * @param {Object} options - Request options
     * @returns {Promise<{browser, page, browserId}>}
     */
    async getPage(options = {}) {
        if (!this.isInitialized) {
            await this.initialize();
        }

        if (this.isShuttingDown) {
            throw new Error('BrowserPool is shutting down');
        }

        const startTime = Date.now();
        this.stats.queuedRequests++;
        this.stats.currentConcurrency++;
        this.stats.peakConcurrency = Math.max(this.stats.peakConcurrency, this.stats.currentConcurrency);

        try {
            const result = await this.acquirePage(options);
            
            const waitTime = Date.now() - startTime;
            this.updateWaitTimeStats(waitTime);
            this.stats.totalRequestsServed++;
            this.stats.queuedRequests--;
            
            this.debugLog(`AdvancedBrowserPool: Page acquired in ${waitTime}ms`);
            return result;
            
        } catch (error) {
            this.stats.queuedRequests--;
            this.stats.currentConcurrency--;
            throw error;
        }
    }

    /**
     * Acquire a page with queue management and load balancing
     */
    async acquirePage(options) {
        // Try to get page immediately
        const immediateResult = await this.tryGetPageImmediate();
        if (immediateResult) {
            return immediateResult;
        }

        // Queue the request if no immediate capacity
        return new Promise((resolve, reject) => {
            const queueEntry = {
                resolve,
                reject,
                timestamp: Date.now(),
                priority: options.priority || 0,
                timeout: setTimeout(() => {
                    this.removeFromQueue(queueEntry);
                    reject(new Error('Request timeout in queue'));
                }, this.config.queueTimeout)
            };

            // Insert with priority (higher priority first)
            const insertIndex = this.requestQueue.findIndex(entry => entry.priority < queueEntry.priority);
            if (insertIndex === -1) {
                this.requestQueue.push(queueEntry);
            } else {
                this.requestQueue.splice(insertIndex, 0, queueEntry);
            }

            this.debugLog(`AdvancedBrowserPool: Request queued (position: ${this.requestQueue.length})`);
        });
    }

    /**
     * Try to get a page immediately without queuing
     */
    async tryGetPageImmediate() {
        // Find browser with available capacity
        const browserId = this.findAvailableBrowser();
        if (!browserId) {
            // Try to create new browser if under limit
            if (this.browsers.size < this.config.maxBrowsers) {
                try {
                    const newBrowserId = await this.createBrowser();
                    return await this.createPageForBrowser(newBrowserId);
                } catch (error) {
                    this.debugLog('AdvancedBrowserPool: Failed to create new browser:', error.message);
                    return null;
                }
            }
            return null;
        }

        return await this.createPageForBrowser(browserId);
    }

    /**
     * Find browser with available capacity using load balancing
     */
    findAvailableBrowser() {
        let bestBrowser = null;
        let lowestLoad = Infinity;

        for (const [browserId, browserInfo] of this.browsers.entries()) {
            if (!browserInfo.isHealthy) continue;
            
            const currentLoad = browserInfo.pages.size;
            if (currentLoad < this.config.maxPagesPerBrowser && currentLoad < lowestLoad) {
                lowestLoad = currentLoad;
                bestBrowser = browserId;
            }
        }

        return bestBrowser;
    }

    /**
     * Create a new page for specific browser
     */
    async createPageForBrowser(browserId) {
        const browserInfo = this.browsers.get(browserId);
        if (!browserInfo || !browserInfo.isHealthy) {
            throw new Error(`Browser ${browserId} not available`);
        }

        try {
            const page = await browserInfo.browser.newPage();
            const pageId = `${browserId}-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
            
            browserInfo.pages.add(pageId);
            browserInfo.lastUsed = Date.now();
            this.stats.totalPagesCreated++;

            // Setup page cleanup on close
            page.on('close', () => {
                browserInfo.pages.delete(pageId);
                this.stats.currentConcurrency--;
                this.processQueue(); // Process next queued request
            });

            this.debugLog(`AdvancedBrowserPool: Created page ${pageId} for browser ${browserId}`);
            
            return {
                browser: browserInfo.browser,
                page,
                browserId,
                pageId,
                releasePage: () => this.releasePage(browserId, pageId)
            };

        } catch (error) {
            // Mark browser as unhealthy if page creation fails
            browserInfo.isHealthy = false;
            this.stats.unhealthyBrowsers++;
            this.stats.healthyBrowsers--;
            throw error;
        }
    }

    /**
     * Release a page back to the pool
     */
    async releasePage(browserId, pageId) {
        const browserInfo = this.browsers.get(browserId);
        if (browserInfo) {
            browserInfo.pages.delete(pageId);
            browserInfo.lastUsed = Date.now();
        }
        
        this.stats.currentConcurrency--;
        this.processQueue();
    }

    /**
     * Process queued requests
     */
    async processQueue() {
        if (this.requestQueue.length === 0) return;

        const queueEntry = this.requestQueue.shift();
        if (!queueEntry) return;

        clearTimeout(queueEntry.timeout);

        try {
            const result = await this.tryGetPageImmediate();
            if (result) {
                queueEntry.resolve(result);
            } else {
                // Put back in queue if still no capacity
                this.requestQueue.unshift(queueEntry);
            }
        } catch (error) {
            queueEntry.reject(error);
        }
    }

    /**
     * Create a new browser instance
     */
    async createBrowser() {
        const browserId = `browser-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
        
        try {
            const browser = await puppeteer.launch({
                headless: 'new',
                args: [
                    '--no-sandbox',
                    '--disable-setuid-sandbox',
                    '--disable-dev-shm-usage',
                    '--disable-blink-features=AutomationControlled',
                    '--disable-features=VizDisplayCompositor',
                    '--disable-background-timer-throttling',
                    '--disable-backgrounding-occluded-windows',
                    '--disable-renderer-backgrounding',
                    '--max_old_space_size=4096'
                ],
                ignoreDefaultArgs: ['--enable-automation'],
                ignoreHTTPSErrors: true,
                timeout: this.config.browserTimeout,
                defaultViewport: { width: 1100, height: 639 }
            });

            const browserInfo = {
                browser,
                pages: new Set(),
                lastUsed: Date.now(),
                isHealthy: true,
                createdAt: Date.now()
            };

            this.browsers.set(browserId, browserInfo);
            this.availableBrowsers.add(browserId);
            this.stats.totalBrowsersCreated++;
            this.stats.healthyBrowsers++;

            // Handle browser disconnect
            browser.on('disconnected', () => {
                this.handleBrowserDisconnect(browserId);
            });

            this.debugLog(`AdvancedBrowserPool: Created browser ${browserId}`);
            this.emit('browserCreated', { browserId, totalBrowsers: this.browsers.size });
            
            return browserId;

        } catch (error) {
            console.error(`AdvancedBrowserPool: Failed to create browser ${browserId}:`, error.message);
            throw error;
        }
    }

    /**
     * Handle browser disconnect
     */
    handleBrowserDisconnect(browserId) {
        const browserInfo = this.browsers.get(browserId);
        if (browserInfo) {
            browserInfo.isHealthy = false;
            this.availableBrowsers.delete(browserId);
            this.stats.unhealthyBrowsers++;
            this.stats.healthyBrowsers--;

            console.warn(`AdvancedBrowserPool: Browser ${browserId} disconnected`);
            this.emit('browserDisconnected', { browserId });
        }
    }

    /**
     * Start health monitoring
     */
    startHealthMonitoring() {
        this.healthCheckTimer = setInterval(async () => {
            await this.performHealthCheck();
            await this.cleanupIdleBrowsers();
        }, this.config.healthCheckInterval);
    }

    /**
     * Perform health check on all browsers
     */
    async performHealthCheck() {
        const healthPromises = [];

        for (const [browserId, browserInfo] of this.browsers.entries()) {
            healthPromises.push(this.checkBrowserHealth(browserId, browserInfo));
        }

        await Promise.allSettled(healthPromises);
    }

    /**
     * Check health of individual browser
     */
    async checkBrowserHealth(browserId, browserInfo) {
        try {
            if (!browserInfo.browser.isConnected()) {
                throw new Error('Browser disconnected');
            }

            // Try to get browser version as health check
            await browserInfo.browser.version();

            if (!browserInfo.isHealthy) {
                browserInfo.isHealthy = true;
                this.availableBrowsers.add(browserId);
                this.stats.healthyBrowsers++;
                this.stats.unhealthyBrowsers--;
                this.debugLog(`AdvancedBrowserPool: Browser ${browserId} recovered`);
            }

        } catch (error) {
            if (browserInfo.isHealthy) {
                browserInfo.isHealthy = false;
                this.availableBrowsers.delete(browserId);
                this.stats.unhealthyBrowsers++;
                this.stats.healthyBrowsers--;
                console.warn(`AdvancedBrowserPool: Browser ${browserId} marked unhealthy:`, error.message);
            }
        }
    }

    /**
     * Cleanup idle browsers
     */
    async cleanupIdleBrowsers() {
        const now = Date.now();
        const browsersToRemove = [];

        for (const [browserId, browserInfo] of this.browsers.entries()) {
            const isIdle = (now - browserInfo.lastUsed) > this.config.idleTimeout;
            const hasNoPages = browserInfo.pages.size === 0;
            const isAboveMinimum = this.browsers.size > this.config.minBrowsers;

            if (isIdle && hasNoPages && isAboveMinimum) {
                browsersToRemove.push(browserId);
            }
        }

        for (const browserId of browsersToRemove) {
            await this.removeBrowser(browserId);
        }
    }

    /**
     * Remove browser from pool
     */
    async removeBrowser(browserId) {
        const browserInfo = this.browsers.get(browserId);
        if (!browserInfo) return;

        try {
            // Close all pages first
            for (const pageId of browserInfo.pages) {
                // Pages will be cleaned up by their close event handlers
            }

            await browserInfo.browser.close();
            this.browsers.delete(browserId);
            this.availableBrowsers.delete(browserId);

            if (browserInfo.isHealthy) {
                this.stats.healthyBrowsers--;
            } else {
                this.stats.unhealthyBrowsers--;
            }

            this.debugLog(`AdvancedBrowserPool: Removed browser ${browserId}`);
            this.emit('browserRemoved', { browserId, totalBrowsers: this.browsers.size });

        } catch (error) {
            console.error(`AdvancedBrowserPool: Error removing browser ${browserId}:`, error.message);
        }
    }

    /**
     * Update wait time statistics
     */
    updateWaitTimeStats(waitTime) {
        this.stats.averageWaitTime = this.stats.averageWaitTime === 0
            ? waitTime
            : (this.stats.averageWaitTime * 0.9 + waitTime * 0.1); // Exponential moving average
    }

    /**
     * Remove request from queue
     */
    removeFromQueue(queueEntry) {
        const index = this.requestQueue.indexOf(queueEntry);
        if (index !== -1) {
            this.requestQueue.splice(index, 1);
        }
    }

    /**
     * Get comprehensive pool statistics
     */
    getStats() {
        return {
            ...this.stats,
            queueLength: this.requestQueue.length,
            totalBrowsers: this.browsers.size,
            availableBrowsers: this.availableBrowsers.size,
            config: this.config,
            isInitialized: this.isInitialized,
            isShuttingDown: this.isShuttingDown
        };
    }

    /**
     * Get detailed browser information
     */
    getBrowserDetails() {
        const details = [];
        for (const [browserId, browserInfo] of this.browsers.entries()) {
            details.push({
                id: browserId,
                isHealthy: browserInfo.isHealthy,
                pageCount: browserInfo.pages.size,
                lastUsed: browserInfo.lastUsed,
                idleTime: Date.now() - browserInfo.lastUsed,
                createdAt: browserInfo.createdAt,
                age: Date.now() - browserInfo.createdAt
            });
        }
        return details;
    }

    /**
     * Graceful shutdown
     */
    async shutdown() {
        if (this.isShuttingDown) return;

        console.log('AdvancedBrowserPool: Starting graceful shutdown...');
        this.isShuttingDown = true;

        // Stop health monitoring
        if (this.healthCheckTimer) {
            clearInterval(this.healthCheckTimer);
        }

        // Reject all queued requests
        for (const queueEntry of this.requestQueue) {
            clearTimeout(queueEntry.timeout);
            queueEntry.reject(new Error('BrowserPool shutting down'));
        }
        this.requestQueue = [];

        // Close all browsers
        const closePromises = [];
        for (const [browserId, browserInfo] of this.browsers.entries()) {
            closePromises.push(
                browserInfo.browser.close().catch(err =>
                    console.error(`Error closing browser ${browserId}:`, err.message)
                )
            );
        }

        await Promise.allSettled(closePromises);
        this.browsers.clear();
        this.availableBrowsers.clear();

        console.log('AdvancedBrowserPool: Shutdown completed');
        this.emit('shutdown');
    }
}

module.exports = AdvancedBrowserPool;
