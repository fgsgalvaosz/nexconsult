const express = require('express');
const cors = require('cors');
const helmet = require('helmet');
const rateLimit = require('express-rate-limit');

// Import routes
const cnpjRoutes = require('./routes/cnpjRoutes');
const healthRoutes = require('./routes/healthRoutes');

// Import middleware
const errorHandler = require('./middleware/errorHandler');
const logger = require('./middleware/logger');

// Import Swagger configuration
const { specs, swaggerUi } = require('./config/swagger');

const app = express();
const PORT = process.env.PORT || 3000;

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

// Rate limiting
const limiter = rateLimit({
    windowMs: 15 * 60 * 1000, // 15 minutes
    max: 100, // limit each IP to 100 requests per windowMs
    message: {
        error: 'Too many requests from this IP, please try again later.'
    }
});

app.use('/api/', limiter);

// Logging middleware
app.use(logger);

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
    console.log(`🚀 CNPJ API Server running on port ${PORT}`);
    console.log(`📖 Documentation available at: http://localhost:${PORT}`);
    console.log(`🔍 API Status: http://localhost:${PORT}/health`);
    console.log(`💡 Environment: ${process.env.NODE_ENV || 'development'}`);
});

// Graceful shutdown
process.on('SIGTERM', () => {
    console.log('SIGTERM received, shutting down gracefully...');
    server.close(() => {
        console.log('Process terminated');
        process.exit(0);
    });
});

process.on('SIGINT', () => {
    console.log('SIGINT received, shutting down gracefully...');
    server.close(() => {
        console.log('Process terminated');
        process.exit(0);
    });
});

module.exports = app;