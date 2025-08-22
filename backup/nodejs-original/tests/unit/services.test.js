const CacheService = require('../../src/services/cacheService');
const PreloadService = require('../../src/services/preloadService');
const ExtractorService = require('../../src/services/extractorService');

describe('Optimized Services Unit Tests', () => {
    
    describe('CacheService', () => {
        let cacheService;
        
        beforeEach(() => {
            cacheService = new CacheService({
                defaultTTL: 1000,
                maxSize: 5,
                cleanupInterval: 100
            });
        });
        
        afterEach(() => {
            cacheService.destroy();
        });

        test('should set and get values correctly', () => {
            cacheService.set('test-key', 'test-value');
            const value = cacheService.get('test-key');
            expect(value).toBe('test-value');
        });

        test('should handle TTL expiration', async () => {
            cacheService.set('expire-key', 'expire-value', 100);
            expect(cacheService.get('expire-key')).toBe('expire-value');
            
            // Wait for expiration
            await new Promise(resolve => setTimeout(resolve, 150));
            expect(cacheService.get('expire-key')).toBeNull();
        });

        test('should evict LRU entries when max size reached', () => {
            // Fill cache to max size
            for (let i = 0; i < 5; i++) {
                cacheService.set(`key-${i}`, `value-${i}`);
            }
            
            // Add one more to trigger eviction
            cacheService.set('new-key', 'new-value');
            
            // First key should be evicted
            expect(cacheService.get('key-0')).toBeNull();
            expect(cacheService.get('new-key')).toBe('new-value');
        });

        test('should provide accurate statistics', () => {
            cacheService.set('stats-key', 'stats-value');
            cacheService.get('stats-key'); // Hit
            cacheService.get('non-existent'); // Miss
            
            const stats = cacheService.getStats();
            expect(stats.metrics.hits).toBe(1);
            expect(stats.metrics.misses).toBe(1);
            expect(stats.hitRate).toBe('50.00%');
        });

        test('should support getOrSet pattern', async () => {
            const factory = jest.fn(() => Promise.resolve('factory-value'));
            
            // First call should use factory
            const value1 = await cacheService.getOrSet('factory-key', factory);
            expect(value1).toBe('factory-value');
            expect(factory).toHaveBeenCalledTimes(1);
            
            // Second call should use cache
            const value2 = await cacheService.getOrSet('factory-key', factory);
            expect(value2).toBe('factory-value');
            expect(factory).toHaveBeenCalledTimes(1); // Not called again
        });
    });

    describe('PreloadService', () => {
        let preloadService;
        
        beforeEach(() => {
            preloadService = new (require('../../src/services/preloadService').constructor)();
        });

        test('should cache resources with TTL', () => {
            preloadService.setCache('resource-key', { data: 'resource-data' }, 1000);
            
            const cached = preloadService.getFromCache('resource-key');
            expect(cached).toBeTruthy();
            expect(cached.data.data).toBe('resource-data');
        });

        test('should evict oldest cache entries when max size reached', () => {
            // Set max size to 3 for testing
            preloadService.config.maxCacheSize = 3;
            
            // Fill cache
            preloadService.setCache('key1', 'value1');
            preloadService.setCache('key2', 'value2');
            preloadService.setCache('key3', 'value3');
            
            // Add one more to trigger eviction
            preloadService.setCache('key4', 'value4');
            
            // First key should be evicted
            expect(preloadService.getFromCache('key1')).toBeNull();
            expect(preloadService.getFromCache('key4')).toBeTruthy();
        });

        test('should provide comprehensive statistics', () => {
            preloadService.setCache('stats-key', 'stats-value');
            preloadService.getFromCache('stats-key'); // Hit
            preloadService.getFromCache('non-existent'); // Miss
            
            const stats = preloadService.getStats();
            expect(stats.cache).toBeDefined();
            expect(stats.preloading).toBeDefined();
            expect(stats.config).toBeDefined();
        });

        test('should clean up expired cache entries', async () => {
            preloadService.setCache('expire-key', 'expire-value', 50);
            expect(preloadService.getFromCache('expire-key')).toBeTruthy();
            
            // Wait for expiration
            await new Promise(resolve => setTimeout(resolve, 100));
            
            const cleanedCount = preloadService.cleanupCache();
            expect(cleanedCount).toBe(1);
            expect(preloadService.getFromCache('expire-key')).toBeNull();
        });
    });

    describe('ExtractorService', () => {
        test('should initialize with DOM cache', () => {
            expect(ExtractorService.domCache).toBeDefined();
            expect(ExtractorService.fontElementsCache).toBeNull();
            expect(ExtractorService.currentDocumentHash).toBeNull();
        });

        test('should provide memory statistics', () => {
            const memStats = ExtractorService.getMemoryStats();
            
            expect(memStats).toHaveProperty('heapUsed');
            expect(memStats).toHaveProperty('heapTotal');
            expect(memStats).toHaveProperty('cacheSize');
            expect(memStats).toHaveProperty('hasFontCache');
            expect(memStats).toHaveProperty('hasDOM');
        });

        test('should clear DOM cache properly', () => {
            // Add some test data to cache
            ExtractorService.domCache.set('test', 'value');
            ExtractorService.fontElementsCache = ['test'];
            ExtractorService.currentDocumentHash = 'test-hash';
            
            ExtractorService.clearDOMCache();
            
            expect(ExtractorService.domCache.size).toBe(0);
            expect(ExtractorService.fontElementsCache).toBeNull();
            expect(ExtractorService.currentDocumentHash).toBeNull();
        });

        test('should create document hash consistently', () => {
            const html1 = '<html><body>Test content</body></html>';
            const html2 = '<html><body>Test content</body></html>';
            const html3 = '<html><body>Different content</body></html>';
            
            const hash1 = ExtractorService.createDocumentHash(html1);
            const hash2 = ExtractorService.createDocumentHash(html2);
            const hash3 = ExtractorService.createDocumentHash(html3);
            
            expect(hash1).toBe(hash2); // Same content should have same hash
            expect(hash1).not.toBe(hash3); // Different content should have different hash
        });

        test('should clean text properly', () => {
            const dirtyText = '  Multiple   spaces   and   ***asterisks***  ';
            const cleanText = ExtractorService.limparTexto(dirtyText);
            
            expect(cleanText).toBe('Multiple spaces and asterisks');
        });
    });

    describe('Integration Tests', () => {
        test('should work together for caching extracted data', async () => {
            const cacheService = new CacheService({ defaultTTL: 5000 });
            const testData = {
                cnpj: '12.345.678/0001-90',
                nomeEmpresarial: 'Test Company',
                situacao: 'ATIVA'
            };
            
            // Cache the data
            cacheService.set('test-cnpj', testData);
            
            // Retrieve and verify
            const cached = cacheService.get('test-cnpj');
            expect(cached).toEqual(testData);
            
            // Check stats
            const stats = cacheService.getStats();
            expect(stats.metrics.hits).toBe(1);
            
            cacheService.destroy();
        });

        test('should handle memory cleanup across services', () => {
            const initialMemory = process.memoryUsage();
            
            // Create and use services
            const cache = new CacheService();
            const preload = new (require('../../src/services/preloadService').constructor)();
            
            // Add some data
            cache.set('memory-test', 'large-data'.repeat(1000));
            preload.setCache('memory-test', 'large-data'.repeat(1000));
            
            // Clean up
            cache.destroy();
            preload.clearAll();
            ExtractorService.clearDOMCache();
            
            // Force garbage collection if available
            if (global.gc) {
                global.gc();
            }
            
            const finalMemory = process.memoryUsage();
            const memoryIncrease = finalMemory.heapUsed - initialMemory.heapUsed;
            
            // Memory increase should be reasonable (less than 10MB for test data)
            expect(memoryIncrease).toBeLessThan(10 * 1024 * 1024);
        });
    });
});
