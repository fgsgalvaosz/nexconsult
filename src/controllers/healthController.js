class HealthController {
    // Root endpoint - API information
    getApiInfo(req, res) {
        res.json({
            name: 'CNPJ Consultation API',
            version: '1.0.0',
            description: 'API para consulta e extração de dados de CNPJ da Receita Federal',
            author: 'CNPJ API Team',
            endpoints: {
                'GET /': 'Informações da API',
                'GET /health': 'Status de saúde da API',
                'GET /system': 'Informações detalhadas do sistema',
                'GET /api/cnpj/status': 'Status do serviço CNPJ',
                'POST /api/cnpj/consultar': 'Consultar CNPJ (consulta completa com todos os dados extraídos)'
            },
            documentation: {
                swagger: '/api/docs',
                openapi: 'OpenAPI 3.0.0'
            },
            support: {
                email: 'support@cnpjapi.com',
                github: 'https://github.com/seu-usuario/cnpj-api'
            }
        });
    }

    // Health check endpoint
    getHealth(req, res) {
        const healthCheck = {
            status: 'healthy',
            timestamp: new Date().toISOString(),
            uptime: process.uptime(),
            environment: process.env.NODE_ENV || 'development',
            version: '1.0.0',
            memory: {
                used: Math.round(process.memoryUsage().heapUsed / 1024 / 1024 * 100) / 100,
                total: Math.round(process.memoryUsage().heapTotal / 1024 / 1024 * 100) / 100,
                external: Math.round(process.memoryUsage().external / 1024 / 1024 * 100) / 100
            },
            system: {
                platform: process.platform,
                arch: process.arch,
                nodeVersion: process.version,
                pid: process.pid
            },
            services: {
                database: 'not_applicable',
                external_apis: {
                    receita_federal: 'operational',
                    solve_captcha: 'operational'
                }
            }
        };

        res.json(healthCheck);
    }

    // Detailed system information (for monitoring)
    getSystemInfo(req, res) {
        const systemInfo = {
            application: {
                name: 'CNPJ Consultation API',
                version: '1.0.0',
                environment: process.env.NODE_ENV || 'development',
                uptime: process.uptime(),
                timestamp: new Date().toISOString()
            },
            system: {
                platform: process.platform,
                arch: process.arch,
                nodeVersion: process.version,
                pid: process.pid,
                cpuUsage: process.cpuUsage(),
                memoryUsage: process.memoryUsage()
            },
            performance: {
                eventLoopDelay: process.hrtime(),
                activeHandles: process._getActiveHandles().length,
                activeRequests: process._getActiveRequests().length
            }
        };

        res.json(systemInfo);
    }
}

module.exports = new HealthController();