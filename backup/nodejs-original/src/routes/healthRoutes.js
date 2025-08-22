const express = require('express');
const healthController = require('../controllers/healthController');

const router = express.Router();

/**
 * @swagger
 * /:
 *   get:
 *     summary: Informações da API
 *     description: Retorna informações gerais sobre a API, endpoints disponíveis e documentação
 *     tags: [Informações]
 *     responses:
 *       200:
 *         description: Informações da API
 *         content:
 *           application/json:
 *             schema:
 *               type: object
 *               properties:
 *                 name:
 *                   type: string
 *                   example: CNPJ Consultation API
 *                 version:
 *                   type: string
 *                   example: 1.0.0
 *                 description:
 *                   type: string
 *                 author:
 *                   type: string
 *                 endpoints:
 *                   type: object
 *                 documentation:
 *                   type: object
 *                 support:
 *                   type: object
 */
router.get('/', healthController.getApiInfo);

/**
 * @swagger
 * /health:
 *   get:
 *     summary: Verificação de saúde
 *     description: Endpoint para verificar o status de saúde da API e métricas do sistema
 *     tags: [Saúde]
 *     responses:
 *       200:
 *         description: Status de saúde da API
 *         content:
 *           application/json:
 *             schema:
 *               $ref: '#/components/schemas/HealthCheck'
 */
router.get('/health', healthController.getHealth);

/**
 * @swagger
 * /system:
 *   get:
 *     summary: Informações detalhadas do sistema
 *     description: Retorna informações detalhadas sobre o sistema, performance e recursos
 *     tags: [Sistema]
 *     responses:
 *       200:
 *         description: Informações detalhadas do sistema
 *         content:
 *           application/json:
 *             schema:
 *               type: object
 *               properties:
 *                 application:
 *                   type: object
 *                 system:
 *                   type: object
 *                 performance:
 *                   type: object
 */
router.get('/system', healthController.getSystemInfo);

module.exports = router;