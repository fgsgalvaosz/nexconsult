/**
 * PreloadService - Intelligent pre-loading and caching system
 * Handles resource pre-loading, intelligent caching, and performance optimization
 */
class PreloadService {
    constructor() {
        this.resourceCache = new Map();
        this.preloadQueue = [];
        this.isPreloading = false;
        this.preloadStats = {
            totalPreloads: 0,
            successfulPreloads: 0,
            failedPreloads: 0,
            cacheHits: 0,
            cacheMisses: 0
        };
        
        // Configuration
        this.config = {
            maxCacheSize: 100,
            maxPreloadConcurrency: 3,
            preloadTimeout: 30000,
            cacheTimeout: 10 * 60 * 1000, // 10 minutes
            enablePreloading: true
        };
        
        this.isDevelopment = process.env.NODE_ENV !== 'production';
        this.debugLog = this.isDevelopment ? console.log : () => {};
    }

    /**
     * Pre-load browser page with common setup
     * @param {Object} browserPool - Browser pool instance
     * @param {Object} options - Pre-loading options
     * @returns {Promise<Object>} Pre-loaded page or null
     */
    async preloadBrowserPage(browserPool, options = {}) {
        if (!this.config.enablePreloading) {
            return null;
        }

        const cacheKey = `browser_page_${JSON.stringify(options)}`;
        
        // Check if we have a cached page
        const cached = this.getFromCache(cacheKey);
        if (cached && cached.isValid()) {
            this.preloadStats.cacheHits++;
            this.debugLog('PreloadService: Using cached browser page');
            return cached.data;
        }

        this.preloadStats.cacheMisses++;
        
        try {
            this.debugLog('PreloadService: Pre-loading browser page...');
            const browser = await browserPool.getBrowser();
            const page = await browser.newPage();
            
            // Basic page setup
            await page.setViewport({ width: 1100, height: 639 });
            await page.setJavaScriptEnabled(true);
            await page.setCacheEnabled(true);
            
            // Cache the pre-loaded page
            this.setCache(cacheKey, {
                browser,
                page,
                createdAt: Date.now(),
                isValid: () => !page.isClosed()
            }, this.config.cacheTimeout);
            
            this.preloadStats.successfulPreloads++;
            this.debugLog('PreloadService: Browser page pre-loaded successfully');
            
            return { browser, page };
            
        } catch (error) {
            this.preloadStats.failedPreloads++;
            this.debugLog('PreloadService: Failed to pre-load browser page:', error.message);
            return null;
        }
    }

    /**
     * Pre-load navigation to consultation page
     * @param {Object} page - Puppeteer page
     * @param {string} cnpj - CNPJ to pre-fill
     * @returns {Promise<boolean>} Success status
     */
    async preloadConsultationPage(page, cnpj) {
        if (!this.config.enablePreloading) {
            return false;
        }

        const cacheKey = `consultation_page_${cnpj}`;
        
        try {
            this.debugLog(`PreloadService: Pre-loading consultation page for CNPJ ${cnpj}...`);
            
            // Navigate to consultation page
            const config = require('../config');
            const optimizedUrl = `${config.CONSULTA_URL}?cnpj=${cnpj}`;
            
            await page.goto(optimizedUrl, {
                waitUntil: 'domcontentloaded',
                timeout: this.config.preloadTimeout
            });
            
            // Wait for essential elements
            await Promise.race([
                page.waitForSelector('.h-captcha', { timeout: 10000 }),
                page.waitForSelector('#cnpj', { timeout: 10000 })
            ]);
            
            // Cache page state
            this.setCache(cacheKey, {
                url: page.url(),
                ready: true,
                timestamp: Date.now()
            }, this.config.cacheTimeout);
            
            this.debugLog('PreloadService: Consultation page pre-loaded successfully');
            return true;
            
        } catch (error) {
            this.debugLog('PreloadService: Failed to pre-load consultation page:', error.message);
            return false;
        }
    }

    /**
     * Intelligent cache with TTL and size management
     * @param {string} key - Cache key
     * @param {*} data - Data to cache
     * @param {number} ttl - Time to live in milliseconds
     */
    setCache(key, data, ttl = this.config.cacheTimeout) {
        // Check cache size and evict if necessary
        if (this.resourceCache.size >= this.config.maxCacheSize) {
            this.evictOldestCache();
        }
        
        const cacheEntry = {
            data,
            timestamp: Date.now(),
            ttl,
            expiresAt: Date.now() + ttl,
            accessCount: 0,
            lastAccessed: Date.now()
        };
        
        this.resourceCache.set(key, cacheEntry);
        this.debugLog(`PreloadService: Cached ${key} with TTL ${ttl}ms`);
    }

    /**
     * Get data from cache with TTL validation
     * @param {string} key - Cache key
     * @returns {*} Cached data or null
     */
    getFromCache(key) {
        const entry = this.resourceCache.get(key);
        
        if (!entry) {
            return null;
        }
        
        // Check if expired
        if (Date.now() > entry.expiresAt) {
            this.resourceCache.delete(key);
            this.debugLog(`PreloadService: Cache expired for ${key}`);
            return null;
        }
        
        // Update access statistics
        entry.accessCount++;
        entry.lastAccessed = Date.now();
        
        return entry;
    }

    /**
     * Evict oldest cache entry
     */
    evictOldestCache() {
        let oldestKey = null;
        let oldestTime = Date.now();
        
        for (const [key, entry] of this.resourceCache.entries()) {
            if (entry.lastAccessed < oldestTime) {
                oldestTime = entry.lastAccessed;
                oldestKey = key;
            }
        }
        
        if (oldestKey) {
            this.resourceCache.delete(oldestKey);
            this.debugLog(`PreloadService: Evicted oldest cache entry: ${oldestKey}`);
        }
    }

    /**
     * Pre-load common resources in background
     * @param {Array} resources - Resources to pre-load
     */
    async preloadResources(resources = []) {
        if (!this.config.enablePreloading || this.isPreloading) {
            return;
        }
        
        this.isPreloading = true;
        this.debugLog(`PreloadService: Starting background pre-loading of ${resources.length} resources`);
        
        try {
            // Process resources in batches
            const batches = this.chunkArray(resources, this.config.maxPreloadConcurrency);
            
            for (const batch of batches) {
                await Promise.allSettled(
                    batch.map(resource => this.preloadSingleResource(resource))
                );
            }
            
        } finally {
            this.isPreloading = false;
            this.debugLog('PreloadService: Background pre-loading completed');
        }
    }

    /**
     * Pre-load a single resource
     * @param {Object} resource - Resource configuration
     */
    async preloadSingleResource(resource) {
        try {
            this.preloadStats.totalPreloads++;
            
            switch (resource.type) {
                case 'browser_page':
                    await this.preloadBrowserPage(resource.browserPool, resource.options);
                    break;
                case 'consultation_page':
                    await this.preloadConsultationPage(resource.page, resource.cnpj);
                    break;
                default:
                    this.debugLog(`PreloadService: Unknown resource type: ${resource.type}`);
            }
            
        } catch (error) {
            this.preloadStats.failedPreloads++;
            this.debugLog(`PreloadService: Failed to pre-load resource:`, error.message);
        }
    }

    /**
     * Chunk array into smaller arrays
     * @param {Array} array - Array to chunk
     * @param {number} size - Chunk size
     * @returns {Array} Array of chunks
     */
    chunkArray(array, size) {
        const chunks = [];
        for (let i = 0; i < array.length; i += size) {
            chunks.push(array.slice(i, i + size));
        }
        return chunks;
    }

    /**
     * Clean up expired cache entries
     */
    cleanupCache() {
        const now = Date.now();
        let cleanedCount = 0;
        
        for (const [key, entry] of this.resourceCache.entries()) {
            if (now > entry.expiresAt) {
                this.resourceCache.delete(key);
                cleanedCount++;
            }
        }
        
        if (cleanedCount > 0) {
            this.debugLog(`PreloadService: Cleaned up ${cleanedCount} expired cache entries`);
        }
        
        return cleanedCount;
    }

    /**
     * Get preload and cache statistics
     * @returns {Object} Statistics
     */
    getStats() {
        const cacheStats = {
            size: this.resourceCache.size,
            maxSize: this.config.maxCacheSize,
            hitRate: this.preloadStats.cacheHits + this.preloadStats.cacheMisses > 0
                ? ((this.preloadStats.cacheHits / (this.preloadStats.cacheHits + this.preloadStats.cacheMisses)) * 100).toFixed(2) + '%'
                : '0%'
        };
        
        return {
            preloading: {
                isActive: this.isPreloading,
                enabled: this.config.enablePreloading,
                ...this.preloadStats
            },
            cache: cacheStats,
            config: this.config
        };
    }

    /**
     * Clear all caches
     */
    clearAll() {
        const size = this.resourceCache.size;
        this.resourceCache.clear();
        this.debugLog(`PreloadService: Cleared all caches (${size} entries)`);
        return size;
    }
}

module.exports = new PreloadService();
