const pino = require('pino');
const path = require('path');
const fs = require('fs');

// Criar diretório de logs se não existir
const logsDir = path.join(process.cwd(), 'logs');
if (!fs.existsSync(logsDir)) {
    fs.mkdirSync(logsDir, { recursive: true });
}

// Configuração do Pino baseada no ambiente
const isDevelopment = process.env.NODE_ENV !== 'production';
const logLevel = process.env.LOG_LEVEL || (isDevelopment ? 'debug' : 'info');

// Configuração base do Pino (sem formatters customizados para transports)
const pinoConfig = {
    level: logLevel,
    timestamp: pino.stdTimeFunctions.isoTime,
    serializers: {
        req: pino.stdSerializers.req,
        res: pino.stdSerializers.res,
        err: pino.stdSerializers.err
    }
};

// Configuração para desenvolvimento (console com cores)
const developmentTransport = {
    target: 'pino-pretty',
    options: {
        colorize: true,
        translateTime: 'HH:MM:ss',
        ignore: 'pid,hostname',
        messageFormat: '[{service}] {msg}',
        levelFirst: true,
        singleLine: false
    }
};

// Configuração de transports para produção (arquivos)
const getProductionTransports = () => {
    const today = new Date().toISOString().split('T')[0];

    return [
        // Arquivo combinado (todos os logs)
        {
            target: 'pino/file',
            options: {
                destination: path.join(logsDir, `combined-${today}.log`),
                mkdir: true
            },
            level: 'debug'
        },
        // Arquivo de erros (apenas erros)
        {
            target: 'pino/file',
            options: {
                destination: path.join(logsDir, `error-${today}.log`),
                mkdir: true
            },
            level: 'error'
        },
        // Arquivo de aplicação (info e acima)
        {
            target: 'pino/file',
            options: {
                destination: path.join(logsDir, `app-${today}.log`),
                mkdir: true
            },
            level: 'info'
        }
    ];
};

// Configuração final dos transports
const transports = isDevelopment
    ? [developmentTransport]
    : [...getProductionTransports(), developmentTransport];

// Criar logger principal com Pino
const logger = pino({
    ...pinoConfig,
    transport: {
        targets: transports
    }
});

// Loggers específicos para cada serviço
const cnpjLogger = logger.child({ service: 'cnpj-service' });
const extractorLogger = logger.child({ service: 'extractor-service' });
const apiLogger = logger.child({ service: 'api' });
const performanceLogger = logger.child({ service: 'performance' });
const systemLogger = logger.child({ service: 'system' });

// Função para log de requisições HTTP com Pino
const logRequest = (req, res, next) => {
    const start = Date.now();
    const requestId = Math.random().toString(36).substring(7);

    // Adicionar requestId ao request para correlação
    req.requestId = requestId;

    // Log da requisição
    apiLogger.info({
        requestId,
        method: req.method,
        url: req.url,
        ip: req.ip,
        userAgent: req.get('User-Agent'),
        body: req.method === 'POST' ? req.body : undefined
    }, 'HTTP Request');

    // Interceptar resposta para log
    const originalSend = res.send;
    res.send = function(data) {
        const duration = Date.now() - start;

        apiLogger.info({
            requestId,
            method: req.method,
            url: req.url,
            statusCode: res.statusCode,
            duration,
            contentLength: data ? data.length : 0
        }, 'HTTP Response');

        // Log de performance se demorou muito
        if (duration > 30000) { // 30 segundos
            performanceLogger.warn({
                requestId,
                method: req.method,
                url: req.url,
                duration,
                statusCode: res.statusCode
            }, 'Slow Request Detected');
        }

        return originalSend.call(this, data);
    };

    next();
};

// Função para log de erros não capturados com Pino
const setupErrorHandling = () => {
    // Capturar exceções não tratadas
    process.on('uncaughtException', (error) => {
        systemLogger.fatal({
            err: error,
            type: 'uncaughtException'
        }, 'Uncaught Exception - Process will exit');

        // Dar tempo para escrever o log antes de sair
        setTimeout(() => {
            process.exit(1);
        }, 1000);
    });

    // Capturar promises rejeitadas
    process.on('unhandledRejection', (reason, promise) => {
        systemLogger.error({
            reason: reason,
            promise: promise,
            type: 'unhandledRejection'
        }, 'Unhandled Promise Rejection');
    });

    // Log de sinais do sistema
    process.on('SIGTERM', () => {
        systemLogger.info('SIGTERM received - shutting down gracefully');
    });

    process.on('SIGINT', () => {
        systemLogger.info('SIGINT received - shutting down gracefully');
    });
};

// Função para log de métricas de sistema com Pino
const logSystemMetrics = () => {
    const used = process.memoryUsage();
    const cpuUsage = process.cpuUsage();

    performanceLogger.debug({
        memory: {
            rss: Math.round(used.rss / 1024 / 1024 * 100) / 100,
            heapTotal: Math.round(used.heapTotal / 1024 / 1024 * 100) / 100,
            heapUsed: Math.round(used.heapUsed / 1024 / 1024 * 100) / 100,
            external: Math.round(used.external / 1024 / 1024 * 100) / 100,
            arrayBuffers: Math.round(used.arrayBuffers / 1024 / 1024 * 100) / 100
        },
        cpu: {
            user: cpuUsage.user,
            system: cpuUsage.system
        },
        uptime: Math.round(process.uptime()),
        pid: process.pid
    }, 'System Metrics');
};

// Log de métricas a cada 5 minutos (apenas em produção)
if (!isDevelopment) {
    setInterval(logSystemMetrics, 5 * 60 * 1000);
}

// Função para substituir console.log com logs estruturados do Pino
const createConsoleProxy = (service) => {
    return {
        log: (...args) => {
            const message = args.join(' ');
            const data = { originalConsole: true };

            if (message.includes('❌') || message.includes('Error')) {
                service.error(data, message);
            } else if (message.includes('⚠️') || message.includes('Warning')) {
                service.warn(data, message);
            } else if (message.includes('✅') || message.includes('🚀') || message.includes('⚡')) {
                service.info(data, message);
            } else {
                service.debug(data, message);
            }
        },
        error: (...args) => service.error({ originalConsole: true }, args.join(' ')),
        warn: (...args) => service.warn({ originalConsole: true }, args.join(' ')),
        info: (...args) => service.info({ originalConsole: true }, args.join(' ')),
        debug: (...args) => service.debug({ originalConsole: true }, args.join(' '))
    };
};

// Console proxy para CNPJ Service
const cnpjConsole = createConsoleProxy(cnpjLogger);

// Função para criar correlação de logs
const createCorrelatedLogger = (baseLogger, correlationId) => {
    return baseLogger.child({ correlationId });
};

module.exports = {
    logger,
    cnpjLogger,
    extractorLogger,
    apiLogger,
    performanceLogger,
    systemLogger,
    logRequest,
    setupErrorHandling,
    logSystemMetrics,
    cnpjConsole,
    createCorrelatedLogger
};
