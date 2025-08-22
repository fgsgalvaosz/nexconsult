/**
 * Global error handling middleware
 */
const errorHandler = (error, req, res, next) => {
    console.error('Error occurred:', {
        message: error.message,
        stack: error.stack,
        url: req.url,
        method: req.method,
        ip: req.ip,
        timestamp: new Date().toISOString()
    });

    // Default error response
    let statusCode = 500;
    let message = 'Internal server error';
    let details = null;

    // Handle specific error types
    if (error.name === 'ValidationError') {
        statusCode = 400;
        message = 'Validation error';
        details = error.message;
    } else if (error.name === 'CastError') {
        statusCode = 400;
        message = 'Invalid data format';
        details = error.message;
    } else if (error.code === 'ENOENT') {
        statusCode = 404;
        message = 'File not found';
        details = error.message;
    } else if (error.message.includes('CNPJ')) {
        statusCode = 400;
        message = 'CNPJ validation error';
        details = error.message;
    } else if (error.message.includes('captcha')) {
        statusCode = 503;
        message = 'Captcha service unavailable';
        details = error.message;
    }

    // Send error response
    res.status(statusCode).json({
        error: message,
        message: details || error.message,
        timestamp: new Date().toISOString(),
        path: req.path,
        method: req.method,
        ...(process.env.NODE_ENV === 'development' && {
            stack: error.stack
        })
    });
};

module.exports = errorHandler;