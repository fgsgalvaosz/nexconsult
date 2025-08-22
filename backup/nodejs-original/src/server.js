const express = require('express');
const cors = require('cors');
const helmet = require('helmet');
const rateLimit = require('express-rate-limit');

// Import routes
const cnpjRoutes = require('./routes/cnpjRoutes');
const healthRoutes = require('./routes/healthRoutes');

// Import middleware
const errorHandler = require('./middleware/errorHandler');
const { logger, apiLogger, logRequest, setupErrorHandling, systemLogger } = require('./utils/logger');

// Import Swagger configuration
const { specs, swaggerUi } = require('./config/swagger');

const app = express();
const PORT = process.env.PORT || 3000;

// Setup error handling for uncaught exceptions
setupErrorHandling();

// Security middleware
app.use(helmet({
    contentSecurityPolicy: {
        directives: {
            defaultSrc: ["'self'"],
            styleSrc: ["'self'", "'unsafe-inline'", "https:"],
            scriptSrc: ["'self'", "'unsafe-inline'"],
            imgSrc: ["'self'", "data:", "https:"],
            connectSrc: ["'self'", "http://localhost:3000", "https://localhost:3000"]
        }
    }
}));

// CORS configuration
app.use(cors({
    origin: [
        'http://localhost:3000',
        'http://127.0.0.1:3000',
        'https://localhost:3000',
        'https://127.0.0.1:3000'
    ],
    methods: ['GET', 'POST', 'PUT', 'DELETE', 'OPTIONS'],
    allowedHeaders: ['Content-Type', 'Authorization', 'Accept'],
    credentials: true
}));

// Body parsing middleware
app.use(express.json({ limit: '10mb' }));
app.use(express.urlencoded({ extended: true, limit: '10mb' }));

// Initialize Concurrent CNPJ Service
const ConcurrentCNPJService = require('./services/concurrentCNPJService');
const concurrentCNPJService = new ConcurrentCNPJService({
    minBrowsers: 3,
    maxBrowsers: 15,
    maxPagesPerBrowser: 3,
    maxQueueSize: 200,
    maxConcurrent: 25,
    throttleRate: 20, // requests per second
    baseMaxRequests: 300, // Higher limit for production
    baseWindowMs: 60000 // 1 minute window
});

// Initialize the service
concurrentCNPJService.initialize().then(() => {
    console.log('🚀 ConcurrentCNPJService initialized successfully');

    // Set the concurrent service in the controller
    const cnpjController = require('./controllers/cnpjController');
    cnpjController.constructor.setConcurrentService(concurrentCNPJService);

}).catch(error => {
    console.error('❌ Failed to initialize ConcurrentCNPJService:', error.message);
    console.log('⚠️ Falling back to legacy CNPJ service');
});

// Use adaptive rate limiting instead of fixed rate limiting
app.use('/api/', concurrentCNPJService.getRateLimitMiddleware());

// Logging middleware (usando Winston)
app.use(logRequest);

// Swagger documentation
app.use('/api/docs', swaggerUi.serve);
app.get('/api/docs', (req, res, next) => {
    // Set specific headers for Swagger UI
    res.setHeader('Content-Security-Policy', "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' http://localhost:3000 http://127.0.0.1:3000");
    next();
}, swaggerUi.setup(specs, {
    explorer: true,
    customCss: '.swagger-ui .topbar { display: none }',
    customSiteTitle: 'CNPJ API Documentation',
    swaggerOptions: {
        url: '/api/docs/swagger.json',
        requestInterceptor: (req) => {
            req.headers['Accept'] = 'application/json';
            return req;
        }
    }
}));

// Swagger JSON endpoint
app.get('/api/docs/swagger.json', (req, res) => {
    res.setHeader('Content-Type', 'application/json');
    res.json(specs);
});

// Routes
app.use('/', healthRoutes);
app.use('/api/cnpj', cnpjRoutes);

// 404 handler
app.use('*', (req, res) => {
    res.status(404).json({
        error: 'Route not found',
        message: 'The requested route does not exist',
        availableRoutes: [
            'GET /',
            'GET /health',
            'GET /system',
            'GET /api/docs',
            'GET /api/cnpj/status',
            'POST /api/cnpj/consultar'
        ]
    });
});

// Error handling middleware
app.use(errorHandler);

// Start server
const server = app.listen(PORT, () => {
    logger.info({
        port: PORT,
        environment: process.env.NODE_ENV || 'development',
        documentation: `http://localhost:${PORT}`,
        healthCheck: `http://localhost:${PORT}/health`,
        logsDirectory: './logs/'
    }, '🚀 CNPJ API Server started');

    console.log(`🚀 CNPJ API Server running on port ${PORT}`);
    console.log(`📖 Documentation available at: http://localhost:${PORT}`);
    console.log(`🔍 API Status: http://localhost:${PORT}/health`);
    console.log(`💡 Environment: ${process.env.NODE_ENV || 'development'}`);
    console.log(`📝 Logs directory: ./logs/`);
});

// Graceful shutdown
process.on('SIGTERM', async () => {
    console.log('SIGTERM received, shutting down gracefully...');

    // Shutdown concurrent service first
    if (concurrentCNPJService) {
        try {
            await concurrentCNPJService.shutdown();
            console.log('ConcurrentCNPJService shutdown completed');
        } catch (error) {
            console.error('Error shutting down ConcurrentCNPJService:', error.message);
        }
    }

    server.close(() => {
        console.log('Process terminated');
        process.exit(0);
    });
});

process.on('SIGINT', async () => {
    systemLogger.info('SIGINT received, shutting down gracefully...');
    console.log('SIGINT received, shutting down gracefully...');

    // Shutdown concurrent service first
    if (concurrentCNPJService) {
        try {
            await concurrentCNPJService.shutdown();
            console.log('ConcurrentCNPJService shutdown completed');
        } catch (error) {
            console.error('Error shutting down ConcurrentCNPJService:', error.message);
        }
    }

    server.close(() => {
        systemLogger.info('Server closed, process terminated');
        console.log('Process terminated');
        process.exit(0);
    });
});

module.exports = app;