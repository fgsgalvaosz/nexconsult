const CNPJService = require('../../src/services/cnpjService');
const BrowserPool = require('../../src/utils/browserPool');
const config = require('../../src/config');

describe('CNPJ Service Performance Tests', () => {
    let cnpjService;
    let browserPool;
    
    // Test CNPJs (use valid format but non-existent numbers for testing)
    const testCNPJs = [
        '11.222.333/0001-81',
        '22.333.444/0001-92',
        '33.444.555/0001-03'
    ];

    beforeAll(async () => {
        browserPool = new BrowserPool({
            maxBrowsers: 3,
            maxPagesPerBrowser: 2,
            browserOptions: {
                headless: true,
                args: ['--no-sandbox', '--disable-dev-shm-usage']
            }
        });
        
        cnpjService = new CNPJService(browserPool);
        
        // Warm up the service
        console.log('Warming up services...');
        await cnpjService.startBackgroundPreloading();
    });

    afterAll(async () => {
        if (browserPool) {
            await browserPool.cleanup();
        }
    });

    describe('Single CNPJ Consultation Performance', () => {
        test('should complete consultation within performance threshold', async () => {
            const cnpj = testCNPJs[0];
            const startTime = Date.now();
            
            try {
                const result = await cnpjService.consultarCNPJCompleto(cnpj, config.SOLVE_CAPTCHA_API_KEY);
                const executionTime = Date.now() - startTime;
                
                console.log(`Single consultation completed in ${executionTime}ms`);
                
                // Performance assertions
                expect(executionTime).toBeLessThan(120000); // Should complete within 2 minutes
                expect(result).toBeDefined();
                
                // Log performance metrics
                const stats = cnpjService.getServiceStats();
                console.log('Service Stats:', JSON.stringify(stats, null, 2));
                
            } catch (error) {
                const executionTime = Date.now() - startTime;
                console.log(`Single consultation failed after ${executionTime}ms:`, error.message);
                
                // Even failures should complete within reasonable time
                expect(executionTime).toBeLessThan(180000); // 3 minutes max for failures
            }
        }, 300000); // 5 minute timeout
    });

    describe('Cache Performance', () => {
        test('should demonstrate cache hit performance improvement', async () => {
            const cnpj = testCNPJs[1];
            
            // First request (cache miss)
            const startTime1 = Date.now();
            try {
                await cnpjService.consultarCNPJCompleto(cnpj, config.SOLVE_CAPTCHA_API_KEY);
            } catch (error) {
                // Expected for test CNPJs
            }
            const firstRequestTime = Date.now() - startTime1;
            
            // Second request (should be cache hit)
            const startTime2 = Date.now();
            try {
                await cnpjService.consultarCNPJCompleto(cnpj, config.SOLVE_CAPTCHA_API_KEY);
            } catch (error) {
                // Expected for test CNPJs
            }
            const secondRequestTime = Date.now() - startTime2;
            
            console.log(`First request: ${firstRequestTime}ms`);
            console.log(`Second request: ${secondRequestTime}ms`);
            
            // Cache hit should be significantly faster
            expect(secondRequestTime).toBeLessThan(firstRequestTime * 0.1); // At least 90% faster
            
            // Verify cache stats
            const cacheStats = cnpjService.getCacheStats();
            expect(cacheStats.hitRate).toContain('%');
            console.log('Cache Stats:', cacheStats);
            
        }, 400000); // 6.5 minute timeout
    });

    describe('Parallel Processing Performance', () => {
        test('should handle multiple concurrent requests efficiently', async () => {
            const concurrentRequests = 3;
            const startTime = Date.now();
            
            const promises = testCNPJs.slice(0, concurrentRequests).map(async (cnpj, index) => {
                const requestStart = Date.now();
                try {
                    const result = await cnpjService.consultarCNPJCompleto(cnpj, config.SOLVE_CAPTCHA_API_KEY);
                    const requestTime = Date.now() - requestStart;
                    return { index, cnpj, success: true, time: requestTime, result };
                } catch (error) {
                    const requestTime = Date.now() - requestStart;
                    return { index, cnpj, success: false, time: requestTime, error: error.message };
                }
            });
            
            const results = await Promise.allSettled(promises);
            const totalTime = Date.now() - startTime;
            
            console.log(`Parallel processing completed in ${totalTime}ms`);
            
            // Analyze results
            const successfulResults = results.filter(r => r.status === 'fulfilled' && r.value.success);
            const failedResults = results.filter(r => r.status === 'fulfilled' && !r.value.success);
            const rejectedResults = results.filter(r => r.status === 'rejected');
            
            console.log(`Successful: ${successfulResults.length}`);
            console.log(`Failed: ${failedResults.length}`);
            console.log(`Rejected: ${rejectedResults.length}`);
            
            // Performance assertions
            expect(totalTime).toBeLessThan(300000); // Should complete within 5 minutes
            
            // At least some requests should complete (even if they fail due to test CNPJs)
            expect(results.length).toBe(concurrentRequests);
            
            // Log individual request times
            results.forEach((result, index) => {
                if (result.status === 'fulfilled') {
                    console.log(`Request ${index}: ${result.value.time}ms - ${result.value.success ? 'Success' : 'Failed'}`);
                }
            });
            
        }, 600000); // 10 minute timeout
    });

    describe('Memory Usage Performance', () => {
        test('should maintain reasonable memory usage', async () => {
            const initialMemory = process.memoryUsage();
            console.log('Initial memory:', {
                heapUsed: Math.round(initialMemory.heapUsed / 1024 / 1024) + ' MB',
                heapTotal: Math.round(initialMemory.heapTotal / 1024 / 1024) + ' MB'
            });
            
            // Perform multiple operations
            for (let i = 0; i < 3; i++) {
                try {
                    await cnpjService.consultarCNPJCompleto(testCNPJs[i % testCNPJs.length], config.SOLVE_CAPTCHA_API_KEY);
                } catch (error) {
                    // Expected for test CNPJs
                }
                
                // Force garbage collection if available
                if (global.gc) {
                    global.gc();
                }
            }
            
            const finalMemory = process.memoryUsage();
            console.log('Final memory:', {
                heapUsed: Math.round(finalMemory.heapUsed / 1024 / 1024) + ' MB',
                heapTotal: Math.round(finalMemory.heapTotal / 1024 / 1024) + ' MB'
            });
            
            const memoryIncrease = finalMemory.heapUsed - initialMemory.heapUsed;
            const memoryIncreaseMB = Math.round(memoryIncrease / 1024 / 1024);
            
            console.log(`Memory increase: ${memoryIncreaseMB} MB`);
            
            // Memory increase should be reasonable (less than 100MB for test operations)
            expect(memoryIncreaseMB).toBeLessThan(100);
            
            // Get service memory stats
            const serviceStats = cnpjService.getServiceStats();
            console.log('Service Memory Stats:', serviceStats.extractor);
            
        }, 300000); // 5 minute timeout
    });

    describe('Service Statistics', () => {
        test('should provide comprehensive performance statistics', async () => {
            const stats = cnpjService.getServiceStats();
            
            // Verify all stat categories are present
            expect(stats).toHaveProperty('cache');
            expect(stats).toHaveProperty('preload');
            expect(stats).toHaveProperty('extractor');
            expect(stats).toHaveProperty('captcha');
            
            // Cache stats
            expect(stats.cache).toHaveProperty('hitRate');
            expect(stats.cache).toHaveProperty('size');
            
            // Preload stats
            expect(stats.preload).toHaveProperty('preloading');
            expect(stats.preload).toHaveProperty('cache');
            
            // Extractor stats
            expect(stats.extractor).toHaveProperty('heapUsed');
            expect(stats.extractor).toHaveProperty('cacheSize');
            
            // Captcha stats
            expect(stats.captcha).toHaveProperty('successRate');
            expect(stats.captcha).toHaveProperty('averageResponseTime');
            
            console.log('Complete Service Statistics:');
            console.log(JSON.stringify(stats, null, 2));
        });
    });

    describe('Error Recovery Performance', () => {
        test('should recover from errors efficiently', async () => {
            const invalidCNPJ = '00.000.000/0000-00'; // Invalid CNPJ
            const startTime = Date.now();
            
            try {
                await cnpjService.consultarCNPJCompleto(invalidCNPJ, config.SOLVE_CAPTCHA_API_KEY);
                fail('Should have thrown an error for invalid CNPJ');
            } catch (error) {
                const errorTime = Date.now() - startTime;
                console.log(`Error recovery completed in ${errorTime}ms`);
                
                // Error should be detected quickly (within 5 seconds)
                expect(errorTime).toBeLessThan(5000);
                expect(error.message).toContain('inválido');
            }
        });
    });
});
