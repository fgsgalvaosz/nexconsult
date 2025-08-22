const cnpjService = require('../services/cnpjService');
const { apiLogger } = require('../utils/logger');

// Get concurrent service instance from app locals (set by server.js)
let concurrentCNPJService = null;

class CNPJController {
    // Set concurrent service instance
    static setConcurrentService(service) {
        concurrentCNPJService = service;
    }

    // Consultar CNPJ (consulta completa com extração automática)
    async consultarCNPJ(req, res, next) {
        try {
            const { cnpj, apiKey, priority } = req.body;

            if (!cnpj) {
                return res.status(400).json({
                    error: 'CNPJ é obrigatório',
                    message: 'Por favor, forneça um CNPJ válido no corpo da requisição'
                });
            }

            // Validar formato do CNPJ
            const cnpjValidation = cnpjService.validateCNPJ(cnpj);
            if (!cnpjValidation.valid) {
                return res.status(400).json({
                    error: 'CNPJ inválido',
                    message: cnpjValidation.message
                });
            }

            apiLogger.info({
                cnpj,
                hasApiKey: !!apiKey,
                priority: priority || 'normal',
                ip: req.ip,
                requestId: req.requestId,
                userAgent: req.get('User-Agent')
            }, 'Starting concurrent CNPJ consultation');

            // Use concurrent service if available, fallback to legacy service
            const serviceToUse = concurrentCNPJService || cnpjService;
            const isUsingConcurrentService = !!concurrentCNPJService;

            console.log(`🔄 Processing CNPJ ${cnpj} using ${isUsingConcurrentService ? 'ConcurrentCNPJService' : 'legacy CNPJService'}`);

            // Realizar consulta completa (consulta + extração automática)
            const resultado = isUsingConcurrentService
                ? await concurrentCNPJService.consultarCNPJ(cnpj, apiKey, {
                    priority: priority || 2,
                    timeout: 300000 // 5 minutes
                  })
                : await cnpjService.consultarCNPJ(cnpj, apiKey);
            
            if (resultado) {
                // Check if it's an error response
                if (resultado.success === false) {
                    // Return appropriate status code based on error type
                    const statusCode = resultado.error === 'CAPTCHA_VALIDATION_FAILED' ? 503 : 500;
                    res.status(statusCode).json(resultado);
                } else {
                    res.json(resultado);
                }
            } else {
                res.status(404).json({
                    success: false,
                    error: 'CNPJ_NOT_FOUND',
                    message: 'Não foi possível obter os dados do CNPJ consultado',
                    cnpj: cnpj,
                    timestamp: new Date().toISOString()
                });
            }
            
        } catch (error) {
            next(error);
        }
    }

    // Status do serviço
    async getStatus(req, res) {
        res.json({
            service: 'CNPJ API',
            status: 'online',
            timestamp: new Date().toISOString(),
            uptime: process.uptime(),
            memory: process.memoryUsage(),
            version: '1.0.0',
            features: [
                'Consulta automática de CNPJ',
                'Resolução automática de hCaptcha',
                'Extração completa de dados',
                'Formatação estruturada de resposta'
            ],
            endpoints: {
                'POST /api/cnpj/consultar': 'Consultar CNPJ (consulta completa com todos os dados extraídos)',
                'GET /api/cnpj/status': 'Status do serviço',
                'DELETE /api/cnpj/cache/clear': 'Limpar cache de consultas',
                'GET /api/cnpj/cache/stats': 'Estatísticas do cache',
                'GET /api/cnpj/performance/browser-pool': 'Estatísticas do pool de browsers',
                'POST /api/cnpj/performance/cleanup': 'Limpar pool de browsers',
                'GET /api/cnpj/logs/recent': 'Logs recentes do sistema'
            }
        });
    }

    // Limpar cache
    async clearCache(req, res) {
        try {
            cnpjService.clearCache();
            res.json({
                message: 'Cache limpo com sucesso',
                timestamp: new Date().toISOString()
            });
        } catch (error) {
            res.status(500).json({
                error: 'Erro ao limpar cache',
                message: error.message
            });
        }
    }

    // Estatísticas do cache
    async getCacheStats(req, res) {
        try {
            const stats = cnpjService.getCacheStats();
            res.json({
                cache: stats,
                timestamp: new Date().toISOString()
            });
        } catch (error) {
            res.status(500).json({
                error: 'Erro ao obter estatísticas do cache',
                message: error.message
            });
        }
    }

    // Estatísticas do pool de browsers
    async getBrowserPoolStats(req, res) {
        try {
            const stats = cnpjService.getBrowserPoolStats();
            res.json({
                browserPool: stats,
                timestamp: new Date().toISOString()
            });
        } catch (error) {
            res.status(500).json({
                error: 'Erro ao obter estatísticas do pool de browsers',
                message: error.message
            });
        }
    }

    // Limpar pool de browsers
    async cleanupBrowserPool(req, res) {
        try {
            await cnpjService.cleanupBrowserPool();
            res.json({
                message: 'Pool de browsers limpo com sucesso',
                timestamp: new Date().toISOString()
            });
        } catch (error) {
            res.status(500).json({
                error: 'Erro ao limpar pool de browsers',
                message: error.message
            });
        }
    }

    // Obter logs recentes
    async getRecentLogs(req, res) {
        try {
            const fs = require('fs').promises;
            const path = require('path');

            const level = req.query.level || 'info';
            const limit = parseInt(req.query.limit) || 100;

            // Determinar qual arquivo de log ler
            let logFile;
            switch (level) {
                case 'error':
                    logFile = 'error';
                    break;
                case 'app':
                    logFile = 'app';
                    break;
                default:
                    logFile = 'combined';
            }

            // Obter data atual para o arquivo
            const today = new Date().toISOString().split('T')[0];
            const logPath = path.join(process.cwd(), 'logs', `${logFile}-${today}.log`);

            try {
                const logContent = await fs.readFile(logPath, 'utf-8');
                const lines = logContent.split('\n').filter(line => line.trim());

                // Pegar as últimas N linhas
                const recentLines = lines.slice(-limit);

                // Tentar parsear cada linha como JSON
                const logs = recentLines.map(line => {
                    try {
                        return JSON.parse(line);
                    } catch (e) {
                        return { message: line, timestamp: new Date().toISOString() };
                    }
                }).reverse(); // Mais recentes primeiro

                res.json({
                    success: true,
                    logs: logs,
                    total: logs.length,
                    level: level,
                    file: logPath,
                    timestamp: new Date().toISOString()
                });

            } catch (fileError) {
                res.json({
                    success: true,
                    logs: [],
                    total: 0,
                    level: level,
                    message: 'Log file not found or empty',
                    timestamp: new Date().toISOString()
                });
            }

        } catch (error) {
            res.status(500).json({
                success: false,
                error: 'Erro ao obter logs',
                message: error.message
            });
        }
    }

    // Get concurrent service statistics
    async getConcurrentStats(req, res) {
        try {
            if (!concurrentCNPJService) {
                return res.status(503).json({
                    error: 'Concurrent service not available',
                    message: 'System is running in legacy mode'
                });
            }

            const stats = concurrentCNPJService.getStats();
            res.json({
                timestamp: new Date().toISOString(),
                mode: 'concurrent',
                stats
            });
        } catch (error) {
            res.status(500).json({
                error: 'Erro ao obter estatísticas',
                message: error.message
            });
        }
    }

    // Get system health
    async getSystemHealth(req, res) {
        try {
            if (!concurrentCNPJService) {
                return res.json({
                    status: 'legacy',
                    message: 'Running in legacy mode',
                    timestamp: new Date().toISOString()
                });
            }

            const health = concurrentCNPJService.getHealth();
            res.json({
                timestamp: new Date().toISOString(),
                ...health
            });
        } catch (error) {
            res.status(500).json({
                error: 'Erro ao obter status de saúde',
                message: error.message
            });
        }
    }

    // Get real-time dashboard data
    async getDashboard(req, res) {
        try {
            if (!concurrentCNPJService) {
                return res.status(503).json({
                    error: 'Dashboard not available',
                    message: 'Concurrent service not initialized'
                });
            }

            const dashboard = concurrentCNPJService.getDashboardData();
            res.json({
                timestamp: new Date().toISOString(),
                dashboard
            });
        } catch (error) {
            res.status(500).json({
                error: 'Erro ao obter dados do dashboard',
                message: error.message
            });
        }
    }
}

const controllerInstance = new CNPJController();

module.exports = controllerInstance;