/**
 * CacheService - Intelligent cache management with TTL, metrics and automatic cleanup
 * Handles all caching operations with performance optimizations and memory management
 */
class CacheService {
    constructor(options = {}) {
        this.cache = new Map();
        this.defaultTTL = options.defaultTTL || 30 * 60 * 1000; // 30 minutes default
        this.maxSize = options.maxSize || 1000; // Maximum cache entries
        this.cleanupInterval = options.cleanupInterval || 5 * 60 * 1000; // 5 minutes cleanup
        this.metrics = {
            hits: 0,
            misses: 0,
            sets: 0,
            deletes: 0,
            cleanups: 0,
            evictions: 0
        };
        
        // Start automatic cleanup
        this.startAutomaticCleanup();
        
        this.isDevelopment = process.env.NODE_ENV !== 'production';
        this.debugLog = this.isDevelopment ? console.log : () => {};
    }

    /**
     * Get value from cache with TTL validation
     */
    get(key) {
        const entry = this.cache.get(key);
        
        if (!entry) {
            this.metrics.misses++;
            this.debugLog(`CacheService: Cache MISS for key: ${key}`);
            return null;
        }

        // Check if entry has expired
        if (this.isExpired(entry)) {
            this.cache.delete(key);
            this.metrics.misses++;
            this.debugLog(`CacheService: Cache EXPIRED for key: ${key}`);
            return null;
        }

        // Update access time for LRU behavior
        entry.lastAccessed = Date.now();
        this.metrics.hits++;
        this.debugLog(`CacheService: Cache HIT for key: ${key}`);
        return entry.value;
    }

    /**
     * Set value in cache with TTL
     */
    set(key, value, ttl = null) {
        const now = Date.now();
        const expiresAt = now + (ttl || this.defaultTTL);
        
        // Check if we need to evict entries due to size limit
        if (this.cache.size >= this.maxSize && !this.cache.has(key)) {
            this.evictLRU();
        }

        const entry = {
            value: value,
            createdAt: now,
            lastAccessed: now,
            expiresAt: expiresAt,
            ttl: ttl || this.defaultTTL
        };

        this.cache.set(key, entry);
        this.metrics.sets++;
        this.debugLog(`CacheService: Cache SET for key: ${key}, TTL: ${entry.ttl}ms`);
        
        return true;
    }

    /**
     * Delete specific key from cache
     */
    delete(key) {
        const deleted = this.cache.delete(key);
        if (deleted) {
            this.metrics.deletes++;
            this.debugLog(`CacheService: Cache DELETE for key: ${key}`);
        }
        return deleted;
    }

    /**
     * Check if cache entry has expired
     */
    isExpired(entry) {
        return Date.now() > entry.expiresAt;
    }

    /**
     * Evict least recently used entry
     */
    evictLRU() {
        let oldestKey = null;
        let oldestTime = Date.now();

        for (const [key, entry] of this.cache.entries()) {
            if (entry.lastAccessed < oldestTime) {
                oldestTime = entry.lastAccessed;
                oldestKey = key;
            }
        }

        if (oldestKey) {
            this.cache.delete(oldestKey);
            this.metrics.evictions++;
            this.debugLog(`CacheService: Evicted LRU entry: ${oldestKey}`);
        }
    }

    /**
     * Clean up expired entries
     */
    cleanup() {
        const now = Date.now();
        let cleanedCount = 0;

        for (const [key, entry] of this.cache.entries()) {
            if (this.isExpired(entry)) {
                this.cache.delete(key);
                cleanedCount++;
            }
        }

        this.metrics.cleanups++;
        this.debugLog(`CacheService: Cleanup completed, removed ${cleanedCount} expired entries`);
        return cleanedCount;
    }

    /**
     * Start automatic cleanup process
     */
    startAutomaticCleanup() {
        this.cleanupTimer = setInterval(() => {
            this.cleanup();
        }, this.cleanupInterval);

        // Ensure cleanup timer doesn't prevent process exit
        if (this.cleanupTimer.unref) {
            this.cleanupTimer.unref();
        }
    }

    /**
     * Stop automatic cleanup
     */
    stopAutomaticCleanup() {
        if (this.cleanupTimer) {
            clearInterval(this.cleanupTimer);
            this.cleanupTimer = null;
        }
    }

    /**
     * Clear all cache entries
     */
    clear() {
        const size = this.cache.size;
        this.cache.clear();
        this.debugLog(`CacheService: Cache cleared, removed ${size} entries`);
        return size;
    }

    /**
     * Get cache statistics
     */
    getStats() {
        const now = Date.now();
        let validEntries = 0;
        let expiredEntries = 0;
        let totalSize = 0;

        for (const [, entry] of this.cache.entries()) {
            if (this.isExpired(entry)) {
                expiredEntries++;
            } else {
                validEntries++;
            }
            
            // Estimate memory usage (rough calculation)
            totalSize += JSON.stringify(entry.value).length;
        }

        const hitRate = this.metrics.hits + this.metrics.misses > 0 
            ? (this.metrics.hits / (this.metrics.hits + this.metrics.misses) * 100).toFixed(2)
            : 0;

        return {
            size: this.cache.size,
            validEntries,
            expiredEntries,
            maxSize: this.maxSize,
            estimatedMemoryKB: Math.round(totalSize / 1024),
            hitRate: `${hitRate}%`,
            metrics: { ...this.metrics },
            defaultTTL: this.defaultTTL,
            cleanupInterval: this.cleanupInterval
        };
    }

    /**
     * Get all cache keys (for debugging)
     */
    getKeys() {
        return Array.from(this.cache.keys());
    }

    /**
     * Check if key exists in cache (without affecting metrics)
     */
    has(key) {
        const entry = this.cache.get(key);
        return entry && !this.isExpired(entry);
    }

    /**
     * Get cache entry info (for debugging)
     */
    getEntryInfo(key) {
        const entry = this.cache.get(key);
        if (!entry) return null;

        const now = Date.now();
        return {
            key,
            createdAt: new Date(entry.createdAt).toISOString(),
            lastAccessed: new Date(entry.lastAccessed).toISOString(),
            expiresAt: new Date(entry.expiresAt).toISOString(),
            ttl: entry.ttl,
            isExpired: this.isExpired(entry),
            ageMs: now - entry.createdAt,
            timeToExpireMs: entry.expiresAt - now
        };
    }

    /**
     * Extend TTL for existing entry
     */
    extendTTL(key, additionalTTL) {
        const entry = this.cache.get(key);
        if (!entry || this.isExpired(entry)) {
            return false;
        }

        entry.expiresAt += additionalTTL;
        entry.lastAccessed = Date.now();
        this.debugLog(`CacheService: Extended TTL for key: ${key} by ${additionalTTL}ms`);
        return true;
    }

    /**
     * Get or set pattern - get value, or set it if not exists
     */
    async getOrSet(key, valueFactory, ttl = null) {
        let value = this.get(key);
        
        if (value === null) {
            // Value not in cache, generate it
            if (typeof valueFactory === 'function') {
                value = await valueFactory();
            } else {
                value = valueFactory;
            }
            
            this.set(key, value, ttl);
        }
        
        return value;
    }

    /**
     * Cleanup and destroy cache service
     */
    destroy() {
        this.stopAutomaticCleanup();
        this.clear();
        this.debugLog('CacheService: Service destroyed');
    }
}

module.exports = CacheService;
