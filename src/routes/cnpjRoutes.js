const express = require('express');
const cnpjController = require('../controllers/cnpjController');

const router = express.Router();

/**
 * @swagger
 * /api/cnpj/status:
 *   get:
 *     summary: Status do serviço CNPJ
 *     description: Retorna informações sobre o status e funcionalidades do serviço de consulta CNPJ
 *     tags: [Status]
 *     responses:
 *       200:
 *         description: Status do serviço
 *         content:
 *           application/json:
 *             schema:
 *               type: object
 *               properties:
 *                 service:
 *                   type: string
 *                   example: CNPJ API
 *                 status:
 *                   type: string
 *                   example: online
 *                 timestamp:
 *                   type: string
 *                   format: date-time
 *                 uptime:
 *                   type: number
 *                   example: 3600.5
 *                 memory:
 *                   type: object
 *                 version:
 *                   type: string
 *                   example: 1.0.0
 *                 features:
 *                   type: array
 *                   items:
 *                     type: string
 *                 endpoints:
 *                   type: object
 */
router.get('/status', cnpjController.getStatus);

/**
 * @swagger
 * /api/cnpj/consultar:
 *   post:
 *     summary: Consultar CNPJ completo
 *     description: Realiza consulta completa de CNPJ na Receita Federal com extração automática de todos os dados
 *     tags: [CNPJ]
 *     requestBody:
 *       required: true
 *       content:
 *         application/json:
 *           schema:
 *             $ref: '#/components/schemas/CNPJRequest'
 *           examples:
 *             exemplo1:
 *               summary: Consulta com CNPJ formatado
 *               value:
 *                 cnpj: "38.139.407/0001-77"
 *                 apiKey: "bd238cb2bace2dd234e32a8df23486f1"
 *             exemplo2:
 *               summary: Consulta apenas com CNPJ
 *               value:
 *                 cnpj: "38139407000177"
 *     responses:
 *       200:
 *         description: Consulta realizada com sucesso
 *         content:
 *           application/json:
 *             schema:
 *               $ref: '#/components/schemas/CNPJResponse'
 *       400:
 *         description: Erro de validação (CNPJ inválido)
 *         content:
 *           application/json:
 *             schema:
 *               $ref: '#/components/schemas/Error'
 *             example:
 *               error: "CNPJ inválido"
 *               message: "O CNPJ deve conter 14 dígitos"
 *               timestamp: "2025-08-22T14:00:00.000Z"
 *               path: "/api/cnpj/consultar"
 *               method: "POST"
 *       404:
 *         description: CNPJ não encontrado
 *         content:
 *           application/json:
 *             schema:
 *               $ref: '#/components/schemas/Error'
 *             example:
 *               error: "Dados não encontrados"
 *               message: "Não foi possível obter os dados do CNPJ consultado"
 *       500:
 *         description: Erro interno do servidor
 *         content:
 *           application/json:
 *             schema:
 *               $ref: '#/components/schemas/Error'
 *             example:
 *               error: "Erro interno do servidor"
 *               message: "Ocorreu um erro durante a consulta do CNPJ"
 */
router.post('/consultar', cnpjController.consultarCNPJ);

/**
 * @swagger
 * /api/cnpj/cache/clear:
 *   delete:
 *     summary: Limpar cache de consultas
 *     tags: [Cache]
 *     responses:
 *       200:
 *         description: Cache limpo com sucesso
 */
router.delete('/cache/clear', cnpjController.clearCache);

/**
 * @swagger
 * /api/cnpj/cache/stats:
 *   get:
 *     summary: Obter estatísticas do cache
 *     tags: [Cache]
 *     responses:
 *       200:
 *         description: Estatísticas do cache
 */
router.get('/cache/stats', cnpjController.getCacheStats);

/**
 * @swagger
 * /api/cnpj/performance/browser-pool:
 *   get:
 *     summary: Obter estatísticas do pool de browsers
 *     tags: [Performance]
 *     responses:
 *       200:
 *         description: Estatísticas do pool de browsers
 */
router.get('/performance/browser-pool', cnpjController.getBrowserPoolStats);

/**
 * @swagger
 * /api/cnpj/performance/cleanup:
 *   post:
 *     summary: Limpar pool de browsers
 *     tags: [Performance]
 *     responses:
 *       200:
 *         description: Pool de browsers limpo com sucesso
 */
router.post('/performance/cleanup', cnpjController.cleanupBrowserPool);

/**
 * @swagger
 * /api/cnpj/logs/recent:
 *   get:
 *     summary: Obter logs recentes
 *     tags: [Logs]
 *     parameters:
 *       - in: query
 *         name: level
 *         schema:
 *           type: string
 *           enum: [error, warn, info, debug]
 *         description: Nível de log para filtrar
 *       - in: query
 *         name: limit
 *         schema:
 *           type: integer
 *           default: 100
 *         description: Número máximo de logs para retornar
 *     responses:
 *       200:
 *         description: Logs recentes
 */
router.get('/logs/recent', cnpjController.getRecentLogs);

module.exports = router;