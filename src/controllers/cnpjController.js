const cnpjService = require('../services/cnpjService');
const { apiLogger } = require('../utils/logger');

class CNPJController {
    // Consultar CNPJ (consulta completa com extração automática)
    async consultarCNPJ(req, res, next) {
        try {
            const { cnpj, apiKey } = req.body;
            
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
                ip: req.ip,
                requestId: req.requestId,
                userAgent: req.get('User-Agent')
            }, 'Starting CNPJ consultation');

            // Realizar consulta completa (consulta + extração automática)
            const resultado = await cnpjService.consultarCNPJ(cnpj, apiKey);
            
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
}

module.exports = new CNPJController();