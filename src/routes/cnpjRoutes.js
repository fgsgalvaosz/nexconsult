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

module.exports = router;