const EventEmitter = require('events');
const os = require('os');

/**
 * RealTimeMonitor - Comprehensive real-time monitoring system
 * Tracks concurrency, throughput, system health, and performance metrics
 */
class RealTimeMonitor extends EventEmitter {
    constructor(options = {}) {
        super();
        
        this.config = {
            // Monitoring intervals
            metricsInterval: options.metricsInterval || 1000, // 1 second
            healthCheckInterval: options.healthCheckInterval || 5000, // 5 seconds
            alertCheckInterval: options.alertCheckInterval || 10000, // 10 seconds
            
            // Alert thresholds
            cpuThreshold: options.cpuThreshold || 80, // 80%
            memoryThreshold: options.memoryThreshold || 85, // 85%
            responseTimeThreshold: options.responseTimeThreshold || 5000, // 5 seconds
            errorRateThreshold: options.errorRateThreshold || 0.1, // 10%
            queueSizeThreshold: options.queueSizeThreshold || 100,
            
            // Data retention
            metricsRetention: options.metricsRetention || 300000, // 5 minutes
            maxDataPoints: options.maxDataPoints || 300,
            
            ...options
        };

        // Metrics storage
        this.metrics = {
            // System metrics
            cpu: [],
            memory: [],
            
            // Application metrics
            concurrency: [],
            throughput: [],
            responseTime: [],
            errorRate: [],
            queueSize: [],
            
            // Browser pool metrics
            browserCount: [],
            availableBrowsers: [],
            
            // Request metrics
            totalRequests: 0,
            successfulRequests: 0,
            failedRequests: 0,
            
            // Performance metrics
            averageResponseTime: 0,
            p95ResponseTime: 0,
            p99ResponseTime: 0
        };

        // Current state
        this.currentState = {
            timestamp: Date.now(),
            concurrency: 0,
            throughput: 0,
            systemHealth: 'healthy',
            alerts: [],
            uptime: 0
        };

        // External data sources
        this.dataSources = new Map();
        
        // Alert state
        this.activeAlerts = new Map();
        this.alertHistory = [];
        
        // Start monitoring
        this.startTime = Date.now();
        this.startMonitoring();
        
        this.debugLog = process.env.NODE_ENV !== 'production' ? console.log : () => {};
    }

    /**
     * Register external data source
     */
    registerDataSource(name, dataSource) {
        this.dataSources.set(name, dataSource);
        this.debugLog(`RealTimeMonitor: Registered data source: ${name}`);
    }

    /**
     * Start all monitoring processes
     */
    startMonitoring() {
        // System metrics monitoring
        this.systemMetricsTimer = setInterval(() => {
            this.collectSystemMetrics();
        }, this.config.metricsInterval);

        // Application metrics monitoring
        this.appMetricsTimer = setInterval(() => {
            this.collectApplicationMetrics();
        }, this.config.metricsInterval);

        // Health checks
        this.healthCheckTimer = setInterval(() => {
            this.performHealthCheck();
        }, this.config.healthCheckInterval);

        // Alert monitoring
        this.alertTimer = setInterval(() => {
            this.checkAlerts();
        }, this.config.alertCheckInterval);

        console.log('RealTimeMonitor: Started monitoring');
    }

    /**
     * Collect system metrics (CPU, Memory, etc.)
     */
    collectSystemMetrics() {
        const timestamp = Date.now();
        
        // CPU usage (approximation using load average)
        const loadAvg = os.loadavg()[0];
        const cpuCount = os.cpus().length;
        const cpuUsage = Math.min(100, (loadAvg / cpuCount) * 100);
        
        // Memory usage
        const totalMem = os.totalmem();
        const freeMem = os.freemem();
        const memoryUsage = ((totalMem - freeMem) / totalMem) * 100;
        
        // Store metrics
        this.addMetric('cpu', { timestamp, value: cpuUsage });
        this.addMetric('memory', { timestamp, value: memoryUsage });
        
        // Update current state
        this.currentState.timestamp = timestamp;
        this.currentState.uptime = timestamp - this.startTime;
    }

    /**
     * Collect application metrics from registered data sources
     */
    collectApplicationMetrics() {
        const timestamp = Date.now();
        
        // Collect from registered data sources
        for (const [name, dataSource] of this.dataSources.entries()) {
            try {
                if (typeof dataSource.getStats === 'function') {
                    const stats = dataSource.getStats();
                    this.processDataSourceStats(name, stats, timestamp);
                }
            } catch (error) {
                this.debugLog(`RealTimeMonitor: Error collecting from ${name}:`, error.message);
            }
        }
        
        // Calculate derived metrics
        this.calculateDerivedMetrics();
    }

    /**
     * Process stats from a data source
     */
    processDataSourceStats(sourceName, stats, timestamp) {
        switch (sourceName) {
            case 'browserPool':
                this.addMetric('browserCount', { timestamp, value: stats.totalBrowsers || 0 });
                this.addMetric('availableBrowsers', { timestamp, value: stats.availableBrowsers || 0 });
                this.currentState.concurrency = stats.currentConcurrency || 0;
                break;
                
            case 'requestQueue':
                this.addMetric('queueSize', { timestamp, value: stats.currentQueueSize || 0 });
                this.addMetric('throughput', { timestamp, value: stats.throughput || 0 });
                this.currentState.throughput = stats.throughput || 0;
                break;
                
            case 'circuitBreaker':
                this.addMetric('errorRate', { timestamp, value: stats.errorRate || 0 });
                this.addMetric('responseTime', { timestamp, value: stats.averageResponseTime || 0 });
                break;
                
            case 'cnpjService':
                if (stats.cache) {
                    // Process cache stats
                    this.metrics.totalRequests += stats.cache.metrics?.sets || 0;
                }
                break;
        }
    }

    /**
     * Calculate derived metrics
     */
    calculateDerivedMetrics() {
        // Calculate percentiles for response time
        const recentResponseTimes = this.getRecentMetrics('responseTime', 60000) // Last minute
            .map(m => m.value)
            .filter(v => v > 0)
            .sort((a, b) => a - b);
            
        if (recentResponseTimes.length > 0) {
            this.metrics.averageResponseTime = recentResponseTimes.reduce((sum, val) => sum + val, 0) / recentResponseTimes.length;
            this.metrics.p95ResponseTime = this.calculatePercentile(recentResponseTimes, 0.95);
            this.metrics.p99ResponseTime = this.calculatePercentile(recentResponseTimes, 0.99);
        }
    }

    /**
     * Calculate percentile value
     */
    calculatePercentile(sortedArray, percentile) {
        if (sortedArray.length === 0) return 0;
        const index = Math.ceil(sortedArray.length * percentile) - 1;
        return sortedArray[Math.max(0, index)];
    }

    /**
     * Add metric data point
     */
    addMetric(metricName, dataPoint) {
        if (!this.metrics[metricName]) {
            this.metrics[metricName] = [];
        }
        
        this.metrics[metricName].push(dataPoint);
        
        // Limit data points to prevent memory growth
        if (this.metrics[metricName].length > this.config.maxDataPoints) {
            this.metrics[metricName] = this.metrics[metricName].slice(-this.config.maxDataPoints);
        }
        
        // Clean old data
        this.cleanOldMetrics(metricName);
    }

    /**
     * Clean old metrics outside retention period
     */
    cleanOldMetrics(metricName) {
        const cutoff = Date.now() - this.config.metricsRetention;
        this.metrics[metricName] = this.metrics[metricName].filter(
            point => point.timestamp > cutoff
        );
    }

    /**
     * Get recent metrics for a specific metric
     */
    getRecentMetrics(metricName, timeWindow = 60000) {
        const cutoff = Date.now() - timeWindow;
        return (this.metrics[metricName] || []).filter(
            point => point.timestamp > cutoff
        );
    }

    /**
     * Perform comprehensive health check
     */
    performHealthCheck() {
        const health = {
            timestamp: Date.now(),
            status: 'healthy',
            checks: {},
            score: 100
        };

        // System health checks
        const latestCpu = this.getLatestMetric('cpu');
        const latestMemory = this.getLatestMetric('memory');
        
        health.checks.cpu = {
            status: latestCpu < this.config.cpuThreshold ? 'healthy' : 'warning',
            value: latestCpu,
            threshold: this.config.cpuThreshold
        };
        
        health.checks.memory = {
            status: latestMemory < this.config.memoryThreshold ? 'healthy' : 'warning',
            value: latestMemory,
            threshold: this.config.memoryThreshold
        };

        // Application health checks
        const latestResponseTime = this.getLatestMetric('responseTime');
        const latestErrorRate = this.getLatestMetric('errorRate');
        const latestQueueSize = this.getLatestMetric('queueSize');
        
        health.checks.responseTime = {
            status: latestResponseTime < this.config.responseTimeThreshold ? 'healthy' : 'warning',
            value: latestResponseTime,
            threshold: this.config.responseTimeThreshold
        };
        
        health.checks.errorRate = {
            status: latestErrorRate < this.config.errorRateThreshold ? 'healthy' : 'critical',
            value: latestErrorRate,
            threshold: this.config.errorRateThreshold
        };
        
        health.checks.queueSize = {
            status: latestQueueSize < this.config.queueSizeThreshold ? 'healthy' : 'warning',
            value: latestQueueSize,
            threshold: this.config.queueSizeThreshold
        };

        // Calculate overall health score
        let warningCount = 0;
        let criticalCount = 0;
        
        for (const check of Object.values(health.checks)) {
            if (check.status === 'warning') warningCount++;
            if (check.status === 'critical') criticalCount++;
        }
        
        health.score = Math.max(0, 100 - (warningCount * 10) - (criticalCount * 30));
        
        if (criticalCount > 0) {
            health.status = 'critical';
        } else if (warningCount > 0) {
            health.status = 'warning';
        }
        
        this.currentState.systemHealth = health.status;
        this.emit('healthCheck', health);
    }

    /**
     * Get latest metric value
     */
    getLatestMetric(metricName) {
        const metrics = this.metrics[metricName];
        if (!metrics || metrics.length === 0) return 0;
        return metrics[metrics.length - 1].value;
    }

    /**
     * Check for alert conditions
     */
    checkAlerts() {
        const alerts = [];
        
        // CPU alert
        const cpuValue = this.getLatestMetric('cpu');
        if (cpuValue > this.config.cpuThreshold) {
            alerts.push({
                type: 'cpu_high',
                severity: 'warning',
                message: `High CPU usage: ${cpuValue.toFixed(1)}%`,
                value: cpuValue,
                threshold: this.config.cpuThreshold
            });
        }
        
        // Memory alert
        const memoryValue = this.getLatestMetric('memory');
        if (memoryValue > this.config.memoryThreshold) {
            alerts.push({
                type: 'memory_high',
                severity: 'warning',
                message: `High memory usage: ${memoryValue.toFixed(1)}%`,
                value: memoryValue,
                threshold: this.config.memoryThreshold
            });
        }
        
        // Error rate alert
        const errorRate = this.getLatestMetric('errorRate');
        if (errorRate > this.config.errorRateThreshold) {
            alerts.push({
                type: 'error_rate_high',
                severity: 'critical',
                message: `High error rate: ${(errorRate * 100).toFixed(1)}%`,
                value: errorRate,
                threshold: this.config.errorRateThreshold
            });
        }
        
        // Queue size alert
        const queueSize = this.getLatestMetric('queueSize');
        if (queueSize > this.config.queueSizeThreshold) {
            alerts.push({
                type: 'queue_size_high',
                severity: 'warning',
                message: `High queue size: ${queueSize}`,
                value: queueSize,
                threshold: this.config.queueSizeThreshold
            });
        }
        
        // Process alerts
        this.processAlerts(alerts);
    }

    /**
     * Process and manage alerts
     */
    processAlerts(newAlerts) {
        const timestamp = Date.now();
        
        // Check for new alerts
        for (const alert of newAlerts) {
            const alertKey = alert.type;
            
            if (!this.activeAlerts.has(alertKey)) {
                // New alert
                alert.timestamp = timestamp;
                alert.id = `${alertKey}-${timestamp}`;
                
                this.activeAlerts.set(alertKey, alert);
                this.alertHistory.push({ ...alert, action: 'triggered' });
                
                console.warn(`🚨 ALERT: ${alert.message}`);
                this.emit('alert', alert);
            }
        }
        
        // Check for resolved alerts
        const currentAlertTypes = new Set(newAlerts.map(a => a.type));
        for (const [alertKey, alert] of this.activeAlerts.entries()) {
            if (!currentAlertTypes.has(alertKey)) {
                // Alert resolved
                this.activeAlerts.delete(alertKey);
                this.alertHistory.push({ 
                    ...alert, 
                    action: 'resolved', 
                    resolvedAt: timestamp,
                    duration: timestamp - alert.timestamp
                });
                
                console.log(`✅ RESOLVED: ${alert.message}`);
                this.emit('alertResolved', alert);
            }
        }
        
        this.currentState.alerts = Array.from(this.activeAlerts.values());
    }

    /**
     * Get comprehensive dashboard data
     */
    getDashboardData() {
        return {
            currentState: this.currentState,
            metrics: {
                cpu: this.getRecentMetrics('cpu', 300000), // Last 5 minutes
                memory: this.getRecentMetrics('memory', 300000),
                concurrency: this.getRecentMetrics('concurrency', 300000),
                throughput: this.getRecentMetrics('throughput', 300000),
                responseTime: this.getRecentMetrics('responseTime', 300000),
                errorRate: this.getRecentMetrics('errorRate', 300000),
                queueSize: this.getRecentMetrics('queueSize', 300000)
            },
            summary: {
                averageResponseTime: this.metrics.averageResponseTime,
                p95ResponseTime: this.metrics.p95ResponseTime,
                p99ResponseTime: this.metrics.p99ResponseTime,
                totalRequests: this.metrics.totalRequests,
                successfulRequests: this.metrics.successfulRequests,
                failedRequests: this.metrics.failedRequests
            },
            alerts: {
                active: Array.from(this.activeAlerts.values()),
                recent: this.alertHistory.slice(-10) // Last 10 alerts
            }
        };
    }

    /**
     * Get real-time statistics
     */
    getStats() {
        return {
            uptime: Date.now() - this.startTime,
            currentState: this.currentState,
            activeAlerts: this.activeAlerts.size,
            totalAlerts: this.alertHistory.length,
            dataPoints: Object.keys(this.metrics).reduce((sum, key) => {
                return sum + (Array.isArray(this.metrics[key]) ? this.metrics[key].length : 0);
            }, 0),
            dataSources: Array.from(this.dataSources.keys()),
            config: this.config
        };
    }

    /**
     * Shutdown monitoring
     */
    shutdown() {
        if (this.systemMetricsTimer) clearInterval(this.systemMetricsTimer);
        if (this.appMetricsTimer) clearInterval(this.appMetricsTimer);
        if (this.healthCheckTimer) clearInterval(this.healthCheckTimer);
        if (this.alertTimer) clearInterval(this.alertTimer);
        
        console.log('RealTimeMonitor: Shutdown completed');
        this.emit('shutdown');
    }
}

module.exports = RealTimeMonitor;
