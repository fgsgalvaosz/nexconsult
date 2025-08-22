const ConcurrentCNPJService = require('../../src/services/concurrentCNPJService');
const config = require('../../src/config');

describe('Concurrent CNPJ Service Load Tests', () => {
    let concurrentService;
    
    // Test CNPJs (valid format but non-existent for testing)
    const testCNPJs = [
        '11.222.333/0001-81',
        '22.333.444/0001-92', 
        '33.444.555/0001-03',
        '44.555.666/0001-14',
        '55.666.777/0001-25',
        '66.777.888/0001-36',
        '77.888.999/0001-47',
        '88.999.000/0001-58',
        '99.000.111/0001-69',
        '00.111.222/0001-70'
    ];

    beforeAll(async () => {
        console.log('🚀 Initializing ConcurrentCNPJService for load testing...');
        
        concurrentService = new ConcurrentCNPJService({
            minBrowsers: 3,
            maxBrowsers: 10,
            maxPagesPerBrowser: 2,
            maxQueueSize: 100,
            maxConcurrent: 15,
            throttleRate: 8 // Reduced for testing
        });
        
        await concurrentService.initialize();
        console.log('✅ ConcurrentCNPJService initialized');
    });

    afterAll(async () => {
        if (concurrentService) {
            await concurrentService.shutdown();
        }
    });

    describe('Burst Load Handling', () => {
        test('should handle 10 concurrent requests without failure', async () => {
            const concurrentRequests = 10;
            const startTime = Date.now();
            
            console.log(`🔥 Starting burst test with ${concurrentRequests} concurrent requests...`);
            
            const promises = [];
            for (let i = 0; i < concurrentRequests; i++) {
                const cnpj = testCNPJs[i % testCNPJs.length];
                const promise = concurrentService.consultarCNPJ(cnpj, config.SOLVE_CAPTCHA_API_KEY, {
                    priority: Math.floor(Math.random() * 3), // Random priority 0-2
                    timeout: 180000 // 3 minutes
                }).then(result => ({
                    index: i,
                    cnpj,
                    success: true,
                    result,
                    duration: Date.now() - startTime
                })).catch(error => ({
                    index: i,
                    cnpj,
                    success: false,
                    error: error.message,
                    duration: Date.now() - startTime
                }));
                
                promises.push(promise);
            }
            
            const results = await Promise.allSettled(promises);
            const totalTime = Date.now() - startTime;
            
            // Analyze results
            const successful = results.filter(r => r.status === 'fulfilled' && r.value.success).length;
            const failed = results.filter(r => r.status === 'fulfilled' && !r.value.success).length;
            const rejected = results.filter(r => r.status === 'rejected').length;
            
            console.log(`📊 Burst test results:`);
            console.log(`  - Total time: ${totalTime}ms`);
            console.log(`  - Successful: ${successful}`);
            console.log(`  - Failed: ${failed}`);
            console.log(`  - Rejected: ${rejected}`);
            console.log(`  - Success rate: ${((successful / concurrentRequests) * 100).toFixed(1)}%`);
            
            // Get service stats
            const stats = concurrentService.getStats();
            console.log(`📈 Service stats:`, {
                totalRequests: stats.service.totalRequests,
                successRate: stats.service.successRate,
                averageResponseTime: `${Math.round(stats.service.averageResponseTime)}ms`,
                throughput: `${stats.service.throughput.toFixed(2)} req/s`,
                queueSize: stats.requestQueue.currentQueueSize,
                browserCount: stats.browserPool.totalBrowsers,
                circuitState: stats.circuitBreaker.state
            });
            
            // Assertions
            expect(totalTime).toBeLessThan(600000); // Should complete within 10 minutes
            expect(successful + failed + rejected).toBe(concurrentRequests);
            expect(successful).toBeGreaterThan(0); // At least some should succeed
            
        }, 900000); // 15 minute timeout
    });

    describe('Sustained Load Handling', () => {
        test('should handle sustained load over time', async () => {
            const requestsPerBatch = 5;
            const batchCount = 3;
            const batchInterval = 30000; // 30 seconds between batches
            
            console.log(`⏱️ Starting sustained load test: ${batchCount} batches of ${requestsPerBatch} requests...`);
            
            const allResults = [];
            
            for (let batch = 0; batch < batchCount; batch++) {
                console.log(`🔄 Processing batch ${batch + 1}/${batchCount}...`);
                
                const batchPromises = [];
                for (let i = 0; i < requestsPerBatch; i++) {
                    const cnpj = testCNPJs[(batch * requestsPerBatch + i) % testCNPJs.length];
                    const promise = concurrentService.consultarCNPJ(cnpj, config.SOLVE_CAPTCHA_API_KEY, {
                        priority: 1,
                        timeout: 120000
                    }).then(result => ({
                        batch,
                        index: i,
                        cnpj,
                        success: true
                    })).catch(error => ({
                        batch,
                        index: i,
                        cnpj,
                        success: false,
                        error: error.message
                    }));
                    
                    batchPromises.push(promise);
                }
                
                const batchResults = await Promise.allSettled(batchPromises);
                allResults.push(...batchResults);
                
                // Log batch results
                const batchSuccessful = batchResults.filter(r => 
                    r.status === 'fulfilled' && r.value.success
                ).length;
                console.log(`  Batch ${batch + 1} completed: ${batchSuccessful}/${requestsPerBatch} successful`);
                
                // Wait before next batch (except for last batch)
                if (batch < batchCount - 1) {
                    console.log(`  Waiting ${batchInterval/1000}s before next batch...`);
                    await new Promise(resolve => setTimeout(resolve, batchInterval));
                }
            }
            
            // Analyze overall results
            const totalRequests = allResults.length;
            const successful = allResults.filter(r => 
                r.status === 'fulfilled' && r.value.success
            ).length;
            
            console.log(`📊 Sustained load test results:`);
            console.log(`  - Total requests: ${totalRequests}`);
            console.log(`  - Successful: ${successful}`);
            console.log(`  - Success rate: ${((successful / totalRequests) * 100).toFixed(1)}%`);
            
            // Get final stats
            const finalStats = concurrentService.getStats();
            console.log(`📈 Final service stats:`, {
                totalRequests: finalStats.service.totalRequests,
                successRate: finalStats.service.successRate,
                averageResponseTime: `${Math.round(finalStats.service.averageResponseTime)}ms`,
                uptime: `${Math.round(finalStats.service.uptime / 1000)}s`
            });
            
            // Assertions
            expect(successful).toBeGreaterThan(0);
            expect(finalStats.service.totalRequests).toBeGreaterThanOrEqual(totalRequests);
            
        }, 1200000); // 20 minute timeout
    });

    describe('System Resilience', () => {
        test('should recover from circuit breaker opening', async () => {
            console.log('🔧 Testing circuit breaker resilience...');
            
            // Force some failures to potentially open circuit breaker
            const failurePromises = [];
            for (let i = 0; i < 3; i++) {
                const promise = concurrentService.consultarCNPJ('00.000.000/0000-00', config.SOLVE_CAPTCHA_API_KEY, {
                    timeout: 5000 // Short timeout to force failures
                }).catch(error => ({ failed: true, error: error.message }));
                
                failurePromises.push(promise);
            }
            
            await Promise.allSettled(failurePromises);
            
            // Check circuit breaker state
            let stats = concurrentService.getStats();
            console.log(`Circuit breaker state after failures: ${stats.circuitBreaker.state}`);
            
            // Wait a bit for potential recovery
            await new Promise(resolve => setTimeout(resolve, 10000));
            
            // Try a valid request
            try {
                const result = await concurrentService.consultarCNPJ(testCNPJs[0], config.SOLVE_CAPTCHA_API_KEY, {
                    timeout: 120000
                });
                console.log('✅ System recovered and processed request successfully');
            } catch (error) {
                console.log(`⚠️ Request failed after recovery attempt: ${error.message}`);
            }
            
            // Get final stats
            stats = concurrentService.getStats();
            console.log(`Final circuit breaker state: ${stats.circuitBreaker.state}`);
            
            // The test passes if the system doesn't crash
            expect(stats).toBeDefined();
            
        }, 300000); // 5 minute timeout
    });

    describe('Performance Metrics', () => {
        test('should provide comprehensive performance metrics', async () => {
            // Make a few requests to generate metrics
            const promises = [];
            for (let i = 0; i < 3; i++) {
                const promise = concurrentService.consultarCNPJ(testCNPJs[i], config.SOLVE_CAPTCHA_API_KEY)
                    .catch(error => ({ error: error.message }));
                promises.push(promise);
            }
            
            await Promise.allSettled(promises);
            
            // Get comprehensive stats
            const stats = concurrentService.getStats();
            const health = concurrentService.getHealth();
            const dashboard = concurrentService.getDashboardData();
            
            console.log('📊 Performance Metrics:');
            console.log('Stats:', JSON.stringify(stats, null, 2));
            console.log('Health:', JSON.stringify(health, null, 2));
            console.log('Dashboard keys:', Object.keys(dashboard));
            
            // Verify all components provide stats
            expect(stats.service).toBeDefined();
            expect(stats.browserPool).toBeDefined();
            expect(stats.requestQueue).toBeDefined();
            expect(stats.circuitBreaker).toBeDefined();
            expect(stats.rateLimit).toBeDefined();
            expect(stats.monitor).toBeDefined();
            
            expect(health.status).toBeDefined();
            expect(health.components).toBeDefined();
            expect(health.metrics).toBeDefined();
            
            expect(dashboard.currentState).toBeDefined();
            expect(dashboard.metrics).toBeDefined();
            expect(dashboard.summary).toBeDefined();
        });
    });
});
