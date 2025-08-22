const cnpjService = require('../services/cnpjService');

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
            
            console.log(`Iniciando consulta completa para CNPJ: ${cnpj}`);
            
            // Realizar consulta completa (consulta + extração automática)
            const resultado = await cnpjService.consultarCNPJ(cnpj, apiKey);
            
            if (resultado) {
                res.json(resultado);
            } else {
                res.status(404).json({
                    error: 'Dados não encontrados',
                    message: 'Não foi possível obter os dados do CNPJ consultado',
                    cnpj: cnpj
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
                'GET /api/cnpj/status': 'Status do serviço'
            }
        });
    }
}

module.exports = new CNPJController();